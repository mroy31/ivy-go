
Ivy: a lightweight software bus
===============================

[Ivy](http://www.eei.cena.fr/products/ivy/) is a lightweight
software bus for quick-prototyping protocols. It allows
applications to broadcast information through text messages, with
a subscription mechanism based on regular expressions.

This is the native go implementation.  A lot of materials, including
libraries for many languages are available from the [Ivy Home
Page](http://www.eei.cena.fr/products/ivy/).

For now, only the following functions from the standard API are available
in this implementation:

- `IvyInit`
- `IvyStart`
- `IvyStop`
- `IvyMainLoop`
- `IvyBindMsg`
- `IvyUnbindMsg`
- `IvySendMsg`
- `IvyBindDirectMsg`
- `IvySendDirectMsg`

Example
-------

```go
package main

import (
    "fmt"
    "log"

    "github.com/mroy31/ivy-go"
)

var busId = "127.255.255.255:2010"

func OnMsg(agent ivy.IvyApplication, params []string) {
    fmt.Println("Receive msg: " + params[0])
}

func main() {
    if err := ivy.IvyInit("ivy-go-example", "Ready", 0, nil, nil); err != nil {
        log.Fatalf("Unable to init ivy bus: %v", err)
    }

    ivy.IvyBindMsg(OnMsg, "(.*)")

    if err := ivy.IvyStart(busId); err != nil {
        log.Fatalf("Unable to start ivy bus: %v", err)
    }

    err := ivy.IvyMainLoop()
    if err != nil {
        log.Fatalf("IvyMainLoop ends with an error: %v", err)
    }
}
```

A more complete example is available in the `example` directory.

License
-------

GNU General Public License v3.0 or later

See [COPYING](COPYING) to see the full text.
