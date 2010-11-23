// Copyright 2010, Evan Shaw. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bson

import (
	"bytes"
	"encoding/binary"
	"os"
	"reflect"
	"strconv"
	"time"
)

var order = binary.LittleEndian

type UnsupportedTypeError struct {
	Type reflect.Type
}

func (e *UnsupportedTypeError) String() string {
	return "bson: unsupported type: " + e.Type.String()
}

type Marshaler interface {
	MarshalBSON() (byte, []byte, os.Error)
}

type encodeState struct {
	*bytes.Buffer
}

func (e *encodeState) marshal(val interface{}) (err os.Error) {
	// write a value just to reserve some space
	e.Write([]byte{0, 0, 0, 0})

	v := reflect.NewValue(val)
	p, ok := v.(*reflect.PtrValue)
	for ok {
		v = p.Elem()
		p, ok = v.(*reflect.PtrValue)
	}

	switch v := v.(type) {
	case *reflect.MapValue:
		// check for string keys
		_, ok := v.Type().(*reflect.MapType).Key().(*reflect.StringType)
		if !ok {
			return &UnsupportedTypeError{v.Type()}
		}
		keys := v.Keys()
		for _, key := range keys {
			e.writeKeyVal(key.Interface().(string), v.Elem(key))
		}
	case *reflect.StructValue:
		t := v.Type().(*reflect.StructType)
		l := t.NumField()
		for i := 0; i < l; i++ {
			field := t.Field(i)
			name := field.Tag
			if name == "" {
				name = field.Name
			}
			e.writeKeyVal(name, v.Field(i))
		}
	case nil:
		// do nothing
	default:
		return &UnsupportedTypeError{v.Type()}
	}
	// terminate the element list
	e.WriteByte(0x00)
	b := e.Bytes()
	e.Buffer = bytes.NewBuffer(b)
	l := int32(len(b))
	toWrite := []byte{byte(l), byte(l >> 8), byte(l >> 16), byte(l >> 24)}
	copy(e.Bytes(), toWrite)
	return nil
}

func (e *encodeState) writeBegin(kind byte, key string) os.Error {
	e.WriteByte(kind)
	e.WriteString(key)
	return e.WriteByte(0x00)
}

func (e *encodeState) writeKeyVal(key string, val reflect.Value) os.Error {
	if val == nil {
		return e.writeBegin(0x0A, key)
	}
	switch v := val.Interface().(type) {
	case Marshaler:
		kind, b, err := v.MarshalBSON()
		if err != nil {
			return err
		}
		e.writeBegin(kind, key)
		_, err = e.Write(b)
		return err
	case *time.Time:
		e.writeBegin(0x09, key)
		return binary.Write(e, order, v.Seconds())
	case []byte:
		e.writeBegin(0x05, key)
		l := int32(len(v))
		binary.Write(e, order, l)
		// binary/generic subtype
		e.WriteByte(0x00)
		_, err := e.Write(v)
		return err
	}
	switch v := val.(type) {
	case *reflect.FloatValue:
		e.writeBegin(0x01, key)
		return binary.Write(e, order, v.Get())
	case *reflect.StringValue:
		e.writeBegin(0x02, key)
		s := v.Get()
		l := int32(len(s)) + 1
		binary.Write(e, order, l)
		e.WriteString(s)
		return e.WriteByte(0x00)
	case *reflect.BoolValue:
		e.writeBegin(0x08, key)
		if v.Get() {
			return e.WriteByte(0x01)
		}
		return e.WriteByte(0x00)
	case *reflect.IntValue:
		if v.Type().Size() <= 4 {
			return e.writeInt32(key, int32(v.Get()))
		}
		return e.writeInt64(key, v.Get())
	case *reflect.UintValue:
		if v.Type().Size() <= 4 {
			return e.writeInt32(key, int32(v.Get()))
		}
		return e.writeInt64(key, int64(v.Get()))
	case *reflect.MapValue:
		e.writeBegin(0x03, key)
		keys := v.Keys()
		e2 := &encodeState{bytes.NewBuffer(nil)}
		for _, key := range keys {
			e2.writeKeyVal(key.Interface().(string), v.Elem(key))
		}
		b := e2.Bytes()
		binary.Write(e, order, int32(len(b)+5))
		e.Write(b)
		return e.WriteByte(0x00)
	case reflect.ArrayOrSliceValue:
		e.writeBegin(0x04, key)
		l := v.Len()
		e2 := &encodeState{bytes.NewBuffer(nil)}
		for i := 0; i < l; i++ {
			e2.writeKeyVal(strconv.Itoa(i), v.Elem(i))
		}
		b := e2.Bytes()
		binary.Write(e, order, int32(len(b)+5))
		e.Write(b)
		return e.WriteByte(0x00)
	case *reflect.PtrValue:
		return e.writeKeyVal(key, v.Elem())
	case *reflect.InterfaceValue:
		return e.writeKeyVal(key, v.Elem())
	case *reflect.StructValue:
		e.writeBegin(0x03, key)
		t := v.Type().(*reflect.StructType)
		l := t.NumField()
		for i := 0; i < l; i++ {
			field := t.Field(i)
			name := field.Tag
			if name == "" {
				name = field.Name
			}
			e.writeKeyVal(name, v.Field(i))
		}
		return e.WriteByte(0x00)
	}

	return &UnsupportedTypeError{val.Type()}
}

func (e *encodeState) writeInt32(key string, val int32) os.Error {
	e.writeBegin(0x10, key)
	return binary.Write(e, order, val)
}

func (e *encodeState) writeInt64(key string, val int64) os.Error {
	e.writeBegin(0x12, key)
	return binary.Write(e, order, val)
}

// Marshal returns the BSON encoding of v, where v is a map value with string keys,
// a struct value, or a pointer to either type of value.
//
// Marshal traverses v recursively. If an encountered value implements the
// Marshaler interface, Marshal calls its MarshalBSON method to produce BSON.
//
// Otherwise, Marshal uses the following type-dependent default encodings:
//
// Boolean values encode as BSON booleans.
//
// Floating point values encode as BSON doubles.
//
// 8-bit, 16-bit, and 32-bit integer values encode as BSON int32 values.
//
// 64-bit and implementation-sized integer values encode as BSON int64 values.
//
// String values encode as BSON strings.
//
// Byte slices encode as BSON generic binary data. Other types of BSON binary
// data are not supported.
//
// Array and slice values other than byte slices encode as BSON arrays.
//
// Struct values encode as BSON embedded documents. Each field becomes a member
// of the object. By default the object's key name is the struct field name
// converted to lower case. If the struct has a tag, that will be used instead.
//
// Map values encode as BSON embedded documents. The map's key type must be a
// string; the object keys are used directly as map keys.
//
// Pointer values encode the value pointed to. A nil pointer encodes as the
// BSON null object.
//
// Interface values encode as the value contained in the interface. A nil
// interface value encodes as the null BSON object.
//
// Channel, complex, and function values cannot be encoded in BSON. Attempting
// to encode such a value causes Marshal to return an InvalidTypeError.
//
// BSON cannot represent cyclic data structures and Marshal cannot handle them.
// Passing cyclic structures to Marshal will result in an infinite recursion.
func Marshal(v interface{}) ([]byte, os.Error) {
	e := &encodeState{bytes.NewBuffer(nil)}
	err := e.marshal(v)
	if err != nil {
		return nil, err
	}
	return e.Bytes(), nil
}
