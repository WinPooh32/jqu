package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"
)

var reservedOrder = []string{"time", "level", "trace_id", "dump", "error", "message"}

type buildstr struct {
	builder *strings.Builder

	table   map[string]json.RawMessage
	printed map[string]struct{}

	order       []string
	orderCustom []string
	orderSet    map[string]struct{}

	withFiled bool
	tzLocal   bool
}

func (b *buildstr) Reset() {
	b.builder.Reset()
	for key := range b.table {
		delete(b.table, key)
	}
	for key := range b.printed {
		delete(b.printed, key)
	}
}

func (b *buildstr) Format() string {
	var newFields []string
	for field := range b.table {
		if _, ok := b.orderSet[field]; ok {
			continue
		}
		newFields = append(newFields, field)
	}
	// Append new fields to known columns.
	// Keep fileds order with same input.
	if len(newFields) > 0 {
		b.orderCustom = append(b.orderCustom, newFields...)

		sort.SliceStable(b.orderCustom,
			func(i, j int) bool { return b.orderCustom[i] < b.orderCustom[j] },
		)
	}
	for _, field := range b.order {
		b.writeField(field)
	}
	for _, field := range b.orderCustom {
		b.writeField(field)
	}
	if b.builder.Len() <= 0 {
		return ""
	}
	b.builder.WriteByte('\n')
	return b.builder.String()
}

func (b *buildstr) writeField(field string) {
	if _, ok := b.printed[field]; ok {
		return
	}
	b.printed[field] = struct{}{}
	if value, ok := b.table[field]; ok {
		var fmtstr string
		var valuebts = bytes.TrimSpace(value)

		switch {
		case b.tzLocal && field == "time":
			tt, err := time.Parse(time.RFC3339, string(bytes.Trim(valuebts, `"`)))
			if err != nil {
				fmtstr = "<nil>"
			} else {
				fmtstr = tt.Local().Format(time.RFC3339)
			}
		default:
			fmtstr = b.format(valuebts)
		}

		if b.withFiled {
			b.builder.WriteString(field)
			b.builder.WriteString(": ")
		}
		b.builder.WriteString(fmtstr)
		b.builder.WriteByte('\t')
	}
}

func (b *buildstr) format(raw []byte) string {
	switch {
	case bytes.HasPrefix(raw, []byte{'{'}), bytes.HasPrefix(raw, []byte{'['}):
		return string(raw)
	default:
		var v interface{}
		json.Unmarshal(raw, &v)
		return fmt.Sprint(v)
	}
}

func makeSet(values ...string) map[string]struct{} {
	set := map[string]struct{}{}
	for _, v := range values {
		set[v] = struct{}{}
	}
	return set
}

func main() {
	var (
		withField = flag.Bool("field", false, "Prepend field name to column")
		tzLocal   = flag.Bool("tz-local", false, "Convert time field value to local timezone")
	)
	flag.Parse()

	go func() {
		var sigCh = make(chan os.Signal, 1)
		signal.Notify(sigCh,
			syscall.SIGINT,
			syscall.SIGQUIT,
			syscall.SIGHUP,
			syscall.SIGTERM,
			syscall.SIGPIPE,
		)
		<-sigCh
		os.Exit(0)
	}()

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Split(bufio.ScanLines)

	var fmter = buildstr{
		builder:     &strings.Builder{},
		table:       map[string]json.RawMessage{},
		printed:     map[string]struct{}{},
		order:       reservedOrder,
		orderCustom: []string{},
		orderSet:    makeSet(reservedOrder...),
		withFiled:   *withField,
		tzLocal:     *tzLocal,
	}

	for scanner.Scan() {
		err := json.Unmarshal(scanner.Bytes(), &fmter.table)
		if err != nil {
			os.Stdout.Write(scanner.Bytes())
			os.Stdout.Write([]byte{'\n'})
			os.Stderr.WriteString(err.Error())
			os.Stderr.Write([]byte{'\n'})
			continue
		}

		_, err = os.Stdout.WriteString(fmter.Format())
		if err != nil {
			os.Stderr.WriteString(err.Error())
			os.Stderr.Write([]byte{'\n'})
			os.Exit(-1)
		}

		fmter.Reset()
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(-1)
	}
}
