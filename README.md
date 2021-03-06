Go-BSON
=======

Go-BSON is a [BSON](http://bsonspec.org/) encoder and decoder package for the [Go
programming language](http://golang.org/). It is primarily intended to be used
with [Mongogo](mongogo).

This project is still in development. It's been tested on Arch and Ubuntu Linux for
the amd64 architecture, but there's no reason it shouldn't work on other architectures
as well.

Dependencies
------------

Go-BSON compiles with Go release 2010-10-27 or newer, barring any recent language or
library changes.

Usage
-----

Go-BSON has two main: one that encodes and one that decodes.

Encode:

    doc := bson.Doc{"hello": "world"}
    data, err := bson.Marshal(doc)

Decode:

    // data is a []byte value that contains BSON data
    doc, err := bson.Unmarshal(data)

The package also provides some types that allow encoding of BSON data that
cannot be represented by Go types, including:

    JavaScript
    MaxKey
    MinKey
    ObjectId
    Regexp
    Symbol

See the documentation in the source for more information.

Contributing
------------

Simply use GitHub as usual to create a fork, make your changes, and create a pull
request. Code is expected to be formatted with gofmt and to adhere to the usual Go
conventions -- that is, the conventions used by Go's core libraries.
