# jqu

Simple tool for unpacking json-formatted logs. Fits good to use alongside the log explorer [glogg](https://glogg.bonnefon.org/index.html).

These fields have predefined order: `time, level, trace_id, dump, error, message`.

## Options

```shell
Usage of jqu:
  -field
        Prepend field name to column
  -tz-local
        Convert time field value to local timezone
```

## Example

```shell
echo '{"level":"info","time":"2022-03-21T05:34:58Z","message":"hello jqu"}' | jqu -field -tz-local
```

Output:

```shell
time: 2022-03-21T15:34:58+10:00 level: info     message: hello jqu
```

## Install

```shell
go install github.com/WinPooh32/jqu@latest
```
