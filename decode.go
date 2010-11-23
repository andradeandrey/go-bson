// Copyright 2010, Evan Shaw. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bson

import (
	"bytes"
	"container/vector"
	"encoding/binary"
	"os"
	"reflect"
	"strings"
	"time"
)

type InvalidUnmarshalError struct {
	Type reflect.Type
}

func (e *InvalidUnmarshalError) String() string {
	if e.Type == nil {
		return "bson: Unmarshal(nil)"
	}

	if _, ok := e.Type.(*reflect.PtrType); !ok {
		return "bson: Unmarshal(non-pointer " + e.Type.String() + ")"
	}
	return "bson: Unmarshal(nil " + e.Type.String() + ")"
}

type DecodeError string

func (e DecodeError) String() string {
	return string(e)
}

type decodeState struct {
	*bytes.Buffer
}

func (d *decodeState) decodeDoc(val reflect.Value) os.Error {
	var (
		mv *reflect.MapValue
		sv *reflect.StructValue
	)
	switch v := val.(type) {
	case *reflect.MapValue:
		mv = v
	case *reflect.StructValue:
		sv = v
	default:
		return &InvalidUnmarshalError{v.Type()}
	}

	// discard total length; it doesn't help us
	d.Next(4)
	kind, err := d.ReadByte()
	for kind > 0 && err == nil {
		var key string
		key, err = d.readCString()
		if err != nil {
			break
		}

		var val reflect.Value
		if mv != nil {
			val = reflect.MakeZero(mv.Type().(*reflect.MapType).Elem())
		} else {
			var f reflect.StructField
			found := false
			// look for field with the right tag
			st := sv.Type().(*reflect.StructType)
			for i := 0; i < sv.NumField(); i++ {
				f = st.Field(i)
				if f.Tag == key {
					found = true
					break
				}
			}
			if !found {
				// next, try exact field name match
				f, found = st.FieldByName(key)
			}
			if !found {
				// last, try case-insensitive field name match
				lowKey := strings.ToLower(key)
				f, found = st.FieldByNameFunc(func(s string) bool { return lowKey == strings.ToLower(s) })
			}
			if found && f.PkgPath == "" {
				val = sv.FieldByIndex(f.Index)
			}
		}

		err = d.decodeElem(kind, val)
		if err != nil {
			break
		}
		if mv != nil {
			mv.SetElem(reflect.NewValue(key), val)
		}

		kind, err = d.ReadByte()
	}
	return err
}

func (d *decodeState) readCString() (string, os.Error) {
	b := d.Bytes()
	i := bytes.IndexByte(b, 0)
	if i < 0 {
		return "", DecodeError("unterminated string")
	}
	s := string(b[:i])
	// discard the bytes we used
	d.Next(i + 1)
	return s, nil
}

func (d *decodeState) readString() (string, os.Error) {
	var l int32
	err := binary.Read(d, order, &l)
	if err != nil {
		return "", err
	}
	b := make([]byte, l-1)
	d.Read(b)
	// discard null terminator
	d.ReadByte()
	return string(b), nil
}

func (d *decodeState) decodeElem(kind byte, val *reflect.Value) os.Error {
	switch kind {
	case 0x01:
		// float
		var f float64
		err := binary.Read(d, order, &f)
		return f, err
	case 0x02:
		// string
		return d.readString()
	case 0x03:
		// document
		return d.decodeDoc(val)
	case 0x04:
		// array
		// byte length doesn't help
		d.Next(4)
		var s vector.Vector
		kind, err := d.ReadByte()
		for kind > 0 && err == nil {
			// discard key
			d.readCString()
			err = d.decodeElem(kind)
			s.Push(el)
			kind, err = d.ReadByte()
		}
		return []interface{}(s), err
	case 0x05:
		// binary data
		var l int32
		err := binary.Read(d, order, &l)
		// assuming binary/generic data; discarding actual kind
		d.ReadByte()
		b := make([]byte, l)
		d.Read(b)
		return b, err
	case 0x07:
		// object ID
		var o ObjectId
		_, err := d.Read(o[:])
		return &o, err
	case 0x08:
		// boolean
		b, err := d.ReadByte()
		return b != 0, err
	case 0x09:
		// time
		var t int64
		err := binary.Read(d, order, &t)
		return time.SecondsToUTC(t), err
	case 0x0A:
		// null
		return nil, nil
	case 0x0B:
		// regex
		r, err := d.readCString()
		// discard options
		d.readCString()
		return &Regexp{Expr: r}, err
	case 0x0D:
		// javascript
		j, err := d.readString()
		return &JavaScript{Code: j}, err
	case 0x0E:
		// symbol
		s, err := d.readString()
		return Symbol(s), err
	case 0x0F:
		// javascript w/ scope
		d.Next(4)
		code, err := d.readString()
		if err != nil {
			return nil, err
		}
		scope, err := d.decodeDoc()
		return &JavaScript{code, scope}, err
	case 0x10:
		// int32
		var i int32
		err := binary.Read(d, order, &i)
		return i, err
	case 0x12:
		// int64
		var i int64
		err := binary.Read(d, order, &i)
		return i, err
	case 0x7F:
		// max key
		return MaxKey{}, nil
	case 0xFF:
		// min key
		return MinKey{}, nil
	}
	return nil, DecodeError("unsupported type")
}

func Unmarshal(data []byte, v interface{}) (map[string]interface{}, os.Error) {
	d := &decodeState{bytes.NewBuffer(data)}
	rv := reflect.NewValue(v)
	p, ok := rv.(*reflect.PtrValue)
	if !ok || p.IsNil() {
		return &InvalidUnmarshalError{rv.Type()}
	}
	return d.decodeDoc(p.Elem())
}

func UnmarshalMap(data []byte) (map[string]interface{}, os.Error) {
	m := make(map[string]interface{})
	err := Unmarshal(data, &m)
	return m, err
}
