WebSocket broadcaster for appended file lines
=============================================

[![Go Report Card](https://goreportcard.com/badge/github.com/jeronimoalbi/tail-ws)](https://goreportcard.com/report/github.com/jeronimoalbi/tail-ws)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)

Installation
------------

Install the binary by running:

```
go install github.com/jeronimoalbi/tail-ws/cmd/tail-ws@latest
```

or alternatively:

```
make install
```

Run
---

To start broadcasting appended lines run:

```
tail-ws FILE
```

New lines are broadcasted by default from the address `ws://127.0.0.1:8080`.
