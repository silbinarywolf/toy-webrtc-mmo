package packbuf

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"strconv"
)

func Write(w io.Writer, data interface{}) error {
	err := writeStruct(w, data)
	if err != nil {
		return err
	}
	return err
}

func writeStruct(w io.Writer, value interface{}) error {
	v := reflect.ValueOf(value).Elem()
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		if !field.CanSet() {
			return errors.New("cannot serialize unexported field: " + field.String())
		}
		switch field.Kind() {
		case reflect.Slice:
			sliceLenCompact := int32(field.Len())
			if err := binary.Write(w, binary.LittleEndian, sliceLenCompact); err != nil {
				return err
			}

			// todo: change to field.Type() and ensure it works
			t := reflect.TypeOf(field.Interface())
			sliceLen := field.Len()
			sliceType := t.Elem()
			switch sliceType.Kind() { // type of the slice element
			case reflect.Struct:
				for i := 0; i < sliceLen; i++ {
					v := field.Index(i)
					if err := writeStruct(w, v.Addr().Interface()); err != nil {
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
					if err := writeStruct(w, v.Interface()); err != nil {
						return err
					}
				}
			default:
				switch fieldValue := field.Interface().(type) {
				case []uint8:
					if _, err := w.Write(fieldValue); err != nil {
						return err
					}
				case []uint16:
					for _, v := range fieldValue {
						if err := binary.Write(w, binary.LittleEndian, &v); err != nil {
							return err
						}
					}
				default:
					// custom types or structs must be explicitly typed
					// using calls to reflect.TypeOf on the defined type.
					return errors.New("TODO: writeStruct for type: " + t.Elem().Kind().String())
				}
			}
			continue
		case reflect.Struct:
			if err := writeStruct(w, field.Addr().Interface()); err != nil {
				return err
			}
			continue
		}
		switch fieldValue := field.Interface().(type) {
		case bool:
			bsBack := [1]byte{0}
			bs := bsBack[:]
			if fieldValue {
				bs[0] = 1
			}
			if _, err := w.Write(bs); err != nil {
				return err
			}
		case byte:
			var bsBack [1]byte
			bs := bsBack[:]
			bs[0] = fieldValue
			binary.LittleEndian.PutUint64(bs, uint64(fieldValue))
			if _, err := w.Write(bs); err != nil {
				return err
			}
		case int:
			// NOTE(Jae): 2021-03-07
			// "int" can be 32-bit or 64-bit in Golang spec, so assuming int64 (largest)
			var bsBack [8]byte
			bs := bsBack[:]
			binary.LittleEndian.PutUint64(bs, uint64(fieldValue))
			if _, err := w.Write(bs); err != nil {
				return err
			}
		case int16:
			var bsBack [2]byte
			bs := bsBack[:]
			binary.LittleEndian.PutUint16(bs, uint16(fieldValue))
			if _, err := w.Write(bs); err != nil {
				return err
			}
		case float32:
			var bsBack [4]byte
			bs := bsBack[:]
			binary.LittleEndian.PutUint32(bs, math.Float32bits(fieldValue))
			if _, err := w.Write(bs); err != nil {
				return err
			}
		case int32,
			int64,
			uint16,
			uint64,
			float64:
			if err := binary.Write(w, binary.LittleEndian, fieldValue); err != nil {
				return err
			}
		case string:
			if len(fieldValue) > maxStringSize {
				return errors.New("cannot write string larger than " + strconv.Itoa(maxStringSize))
			}
			if err := binary.Write(w, binary.LittleEndian, uint16(len(fieldValue))); err != nil {
				return err
			}
			// todo(jae): 2021-03-07
			// casts to []byte which could be slow, profile later and make fast
			if _, err := w.Write([]byte(fieldValue)); err != nil {
				return err
			}
		default:
			return fmt.Errorf("Cannot write unsupported data type: %T in packet type %T", fieldValue, value)
		}
	}
	return nil
}
