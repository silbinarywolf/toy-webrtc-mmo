package packbuf

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strconv"
)

const (
	// maxStringSize is the maximum string that can be sent over the wire
	// the number chosen was arbitrary
	maxStringSize = 65535
)

func Read(r io.Reader, data interface{}) error {
	err := readStruct(r, data)
	if err != nil {
		return err
	}
	return nil
}

func readStruct(buf io.Reader, structData interface{}) error {
	v := reflect.ValueOf(structData).Elem()
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		if !field.CanSet() {
			continue
		}

		if t := reflect.TypeOf(field.Interface()); t.Kind() == reflect.Slice {
			var sliceLenCompact int32
			if err := binary.Read(buf, binary.LittleEndian, &sliceLenCompact); err != nil {
				return err
			}

			//
			sliceLen := int(sliceLenCompact)
			if sliceLen == 0 {
				// Ignore setting if no data
				// This ensures the data stays as "nil"
				continue
			}
			// todo(Jae): 2021-04-04
			// allow setting max limit on slice data structure via tags
			// on struct? ie. `packbuf:"maxsize:5"`
			//
			// this would stop the server from being able to receive weird
			// false packets

			slice := reflect.MakeSlice(t, sliceLen, sliceLen)
			field.Set(slice)
			switch sliceType := t.Elem(); sliceType.Kind() {
			case reflect.Uint8:
				value := make([]byte, sliceLen)
				if _, err := buf.Read(value); err != nil {
					return err
				}
				field.SetBytes(value)
			case reflect.Uint16:
				slice := make([]uint16, sliceLen)
				for i := 0; i < sliceLen; i++ {
					if err := binary.Read(buf, binary.LittleEndian, &slice[i]); err != nil {
						return err
					}
				}
				field.Set(reflect.ValueOf(slice))
			case reflect.Struct:
				for i := 0; i < sliceLen; i++ {
					v := field.Index(i)
					if err := readStruct(buf, v.Addr().Interface()); err != nil {
						return err
					}
				}
			case reflect.Ptr:
				ptrToType := sliceType.Elem()
				if ptrToType.Kind() != reflect.Struct {
					return errors.New("unable to handle []*Type where Type is not a struct")
				}
				for i := 0; i < sliceLen; i++ {
					v := field.Index(i)
					v.Set(reflect.New(ptrToType))
					if err := readStruct(buf, v.Interface()); err != nil {
						return err
					}
				}
			default:
				// custom types or structs must be explicitly typed
				// using calls to reflect.TypeOf on the defined type.
				return errors.New("TODO: Read: " + t.Elem().Kind().String())
			}
			continue
		}
		if field.Kind() == reflect.Struct {
			if err := readStruct(buf, field.Addr().Interface()); err != nil {
				return err
			}
			continue
		}
		switch fieldType := field.Interface().(type) {
		case bool:
			var value byte
			if err := binary.Read(buf, binary.LittleEndian, &value); err != nil {
				return err
			}
			if value != 0 {
				field.SetBool(true)
				break
			}
			field.SetBool(false)
		case byte:
			var value byte
			if err := binary.Read(buf, binary.LittleEndian, &value); err != nil {
				return err
			}
			field.SetUint(uint64(value))
		case uint16:
			var value uint16
			if err := binary.Read(buf, binary.LittleEndian, &value); err != nil {
				return err
			}
			field.SetUint(uint64(value))
		case uint64:
			var value uint64
			if err := binary.Read(buf, binary.LittleEndian, &value); err != nil {
				return err
			}
			field.SetUint(value)
		case int,
			int64:
			// NOTE(Jae): 2020-05-16
			// "int" can be 32-bit or 64-bit in Golang spec, so assuming int64 (largest)
			var value int64
			if err := binary.Read(buf, binary.LittleEndian, &value); err != nil {
				return err
			}
			field.SetInt(value)
		case int32:
			var value int32
			if err := binary.Read(buf, binary.LittleEndian, &value); err != nil {
				return err
			}
			field.SetInt(int64(value))
		case float32:
			var value float32
			if err := binary.Read(buf, binary.LittleEndian, &value); err != nil {
				return err
			}
			field.SetFloat(float64(value))
		case float64:
			var value float64
			if err := binary.Read(buf, binary.LittleEndian, &value); err != nil {
				return err
			}
			field.SetFloat(value)
		case string:
			var stringSize uint16
			if err := binary.Read(buf, binary.LittleEndian, &stringSize); err != nil {
				return err
			}
			if stringSize > maxStringSize {
				return errors.New("cannot write string larger than " + strconv.Itoa(maxStringSize))
			}
			if stringSize == 0 {
				// Nothing to write to field
				continue
			}
			stringData := make([]byte, stringSize)
			if _, err := buf.Read(stringData); err != nil {
				return err
			}
			field.SetString(string(stringData))
		default:
			return fmt.Errorf("cannot read unsupported data type: %T in struct %T", fieldType, structData)
		}
	}
	return nil
}
