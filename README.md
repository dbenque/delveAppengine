# Delve Appengine

[![Build Status](https://travis-ci.org/dbenque/delveAppengine.svg?branch=master)](https://travis-ci.org/dbenque/delveAppengine)

This projects should be used to automatically attach Delve debugger to an Appengine module running locally.

```
Usage of delveAppengine:
  -delay int
        Time delay in seconds between each appengine process scan (default 3)
  -key string
        Magic key to identify a specific module bianry (default is empty string)
  -port int
        Port used by the Delve server (default 2345)
```

Tested under Linux (Arch and Ubuntu)

Tested under Mac thanks to [cedriclam](https://github.com/cedriclam)

