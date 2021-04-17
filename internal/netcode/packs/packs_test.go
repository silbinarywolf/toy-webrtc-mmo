// packs is a package that holds the packet data structures
package packs

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"reflect"
	"strings"
	"testing"

	"github.com/silbinarywolf/toy-webrtc-mmo/internal/netcode/rtt"
)

// TestPacketWriteRead will test all the packet types to ensure writing / reading works
// for each registered struct
func TestPacketWriteRead(t *testing.T) {
	writerRtt := &rtt.RoundTripTracking{}
	for _, packetType := range packetIDToType {
		packet := reflect.New(packetType).Interface().(Packet)
		writer := bytes.NewBuffer(nil)
		err := Write(writer, writerRtt, packet)
		if err != nil {
			t.Errorf("Failed Packet.Write: %v\n", err)
			return
		}
		reader := bytes.NewReader(writer.Bytes())
		seqID, packetOutput, err := Read(reader)
		if err != nil {
			t.Errorf("Failed Packet.Read: %v\n", err)
			return
		}
		if diff := DeepEqual(packet, packetOutput); diff != nil {
			t.Errorf("Unable to serialize/deserialize packet: %T (seq id: %d) ---- \n%v", packet, seqID, diff)
			return
		}
	}
}

// TestPacketDoubleWriteAndRead tests that at least two packet types can be written
// and then read without breakage
func TestPacketDoubleWriteAndDoubleRead(t *testing.T) {
	writerRtt := &rtt.RoundTripTracking{}
	for _, packetType := range packetIDToType {
		packet := reflect.New(packetType).Interface().(Packet)
		packetTwo := reflect.New(packetType).Interface().(Packet)
		writer := bytes.NewBuffer(nil)
		if err := Write(writer, writerRtt, packet); err != nil {
			t.Errorf("Failed Packet.Write: %v\n", err)
			return
		}
		if err := Write(writer, writerRtt, packetTwo); err != nil {
			t.Errorf("Failed second Packet.Write: %v\n", err)
			return
		}
		reader := bytes.NewReader(writer.Bytes())
		{
			seqID, packetOutput, err := Read(reader)
			if err != nil {
				t.Errorf("Failed Packet.Read: %v\n", err)
				return
			}
			if diff := DeepEqual(packet, packetOutput); diff != nil {
				t.Errorf("Unable to Read packet: %T (seq id: %d) ---- \n%v", packet, seqID, diff)
				return
			}
		}
		{
			seqID, packetOutput, err := Read(reader)
			if err != nil {
				t.Errorf("Failed Packet.Read: %v\n", err)
				return
			}
			if diff := DeepEqual(packetTwo, packetOutput); diff != nil {
				t.Errorf("Unable to Read packet: %T (seq id: %d) ---- \n%v", packetTwo, seqID, diff)
				return
			}
		}
	}
}

// Below is a copy-paste of
// https://github.com/go-test/deep/commit/8ed16920c079d9f721f068f915e1539e9ef3236c

var (
	// FloatPrecision is the number of decimal places to round float values
	// to when comparing.
	FloatPrecision = 10

	// MaxDiff specifies the maximum number of differences to return.
	MaxDiff = 10

	// MaxDepth specifies the maximum levels of a struct to recurse into,
	// if greater than zero. If zero, there is no limit.
	MaxDepth = 0

	// LogErrors causes errors to be logged to STDERR when true.
	LogErrors = false

	// CompareUnexportedFields causes unexported struct fields, like s in
	// T{s int}, to be compared when true.
	CompareUnexportedFields = false

	// NilSlicesAreEmpty causes a nil slice to be equal to an empty slice.
	NilSlicesAreEmpty = false

	// NilMapsAreEmpty causes a nil map to be equal to an empty map.
	NilMapsAreEmpty = false
)

var (
	// ErrMaxRecursion is logged when MaxDepth is reached.
	ErrMaxRecursion = errors.New("recursed to MaxDepth")

	// ErrTypeMismatch is logged when Equal passed two different types of values.
	ErrTypeMismatch = errors.New("variables are different reflect.Type")

	// ErrNotHandled is logged when a primitive Go kind is not handled.
	ErrNotHandled = errors.New("cannot compare the reflect.Kind")
)

type cmp struct {
	diff        []string
	buff        []string
	floatFormat string
}

var errorType = reflect.TypeOf((*error)(nil)).Elem()

// DeepEqual compares variables a and b, recursing into their structure up to
// MaxDepth levels deep (if greater than zero), and returns a list of differences,
// or nil if there are none. Some differences may not be found if an error is
// also returned.
//
// If a type has an Equal method, like time.Equal, it is called to check for
// equality.
//
// When comparing a struct, if a field has the tag `deep:"-"` then it will be
// ignored.
func DeepEqual(a, b interface{}) []string {
	aVal := reflect.ValueOf(a)
	bVal := reflect.ValueOf(b)
	c := &cmp{
		diff:        []string{},
		buff:        []string{},
		floatFormat: fmt.Sprintf("%%.%df", FloatPrecision),
	}
	if a == nil && b == nil {
		return nil
	} else if a == nil && b != nil {
		c.saveDiff("<nil pointer>", b)
	} else if a != nil && b == nil {
		c.saveDiff(a, "<nil pointer>")
	}
	if len(c.diff) > 0 {
		return c.diff
	}

	c.equals(aVal, bVal, 0)
	if len(c.diff) > 0 {
		return c.diff // diffs
	}
	return nil // no diffs
}

func (c *cmp) equals(a, b reflect.Value, level int) {
	if MaxDepth > 0 && level > MaxDepth {
		logError(ErrMaxRecursion)
		return
	}

	// Check if one value is nil, e.g. T{x: *X} and T.x is nil
	if !a.IsValid() || !b.IsValid() {
		if a.IsValid() && !b.IsValid() {
			c.saveDiff(a.Type(), "<nil pointer>")
		} else if !a.IsValid() && b.IsValid() {
			c.saveDiff("<nil pointer>", b.Type())
		}
		return
	}

	// If different types, they can't be equal
	aType := a.Type()
	bType := b.Type()
	if aType != bType {
		// Built-in types don't have a name, so don't report [3]int != [2]int as " != "
		if aType.Name() == "" || aType.Name() != bType.Name() {
			c.saveDiff(aType, bType)
		} else {
			// Type names can be the same, e.g. pkg/v1.Error and pkg/v2.Error
			// are both exported as pkg, so unless we include the full pkg path
			// the diff will be "pkg.Error != pkg.Error"
			// https://github.com/go-test/deep/issues/39
			aFullType := aType.PkgPath() + "." + aType.Name()
			bFullType := bType.PkgPath() + "." + bType.Name()
			c.saveDiff(aFullType, bFullType)
		}
		logError(ErrTypeMismatch)
		return
	}

	// Primitive https://golang.org/pkg/reflect/#Kind
	aKind := a.Kind()
	bKind := b.Kind()

	// Do a and b have underlying elements? Yes if they're ptr or interface.
	aElem := aKind == reflect.Ptr || aKind == reflect.Interface
	bElem := bKind == reflect.Ptr || bKind == reflect.Interface

	// If both types implement the error interface, compare the error strings.
	// This must be done before dereferencing because the interface is on a
	// pointer receiver. Re https://github.com/go-test/deep/issues/31, a/b might
	// be primitive kinds; see TestErrorPrimitiveKind.
	if aType.Implements(errorType) && bType.Implements(errorType) {
		if (!aElem || !a.IsNil()) && (!bElem || !b.IsNil()) {
			aString := a.MethodByName("Error").Call(nil)[0].String()
			bString := b.MethodByName("Error").Call(nil)[0].String()
			if aString != bString {
				c.saveDiff(aString, bString)
				return
			}
		}
	}

	// Dereference pointers and interface{}
	if aElem || bElem {
		if aElem {
			a = a.Elem()
		}
		if bElem {
			b = b.Elem()
		}
		c.equals(a, b, level+1)
		return
	}

	switch aKind {

	/////////////////////////////////////////////////////////////////////
	// Iterable kinds
	/////////////////////////////////////////////////////////////////////

	case reflect.Struct:
		/*
			The variables are structs like:
				type T struct {
					FirstName string
					LastName  string
				}
			Type = <pkg>.T, Kind = reflect.Struct
			Iterate through the fields (FirstName, LastName), recurse into their values.
		*/

		// Types with an Equal() method, like time.Time, only if struct field
		// is exported (CanInterface)
		if eqFunc := a.MethodByName("Equal"); eqFunc.IsValid() && eqFunc.CanInterface() {
			// Handle https://github.com/go-test/deep/issues/15:
			// Don't call T.Equal if the method is from an embedded struct, like:
			//   type Foo struct { time.Time }
			// First, we'll encounter Equal(Ttime, time.Time) but if we pass b
			// as the 2nd arg we'll panic: "Call using pkg.Foo as type time.Time"
			// As far as I can tell, there's no way to see that the method is from
			// time.Time not Foo. So we check the type of the 1st (0) arg and skip
			// unless it's b type. Later, we'll encounter the time.Time anonymous/
			// embedded field and then we'll have Equal(time.Time, time.Time).
			funcType := eqFunc.Type()
			if funcType.NumIn() == 1 && funcType.In(0) == bType {
				retVals := eqFunc.Call([]reflect.Value{b})
				if !retVals[0].Bool() {
					c.saveDiff(a, b)
				}
				return
			}
		}

		for i := 0; i < a.NumField(); i++ {
			if aType.Field(i).PkgPath != "" && !CompareUnexportedFields {
				continue // skip unexported field, e.g. s in type T struct {s string}
			}

			if aType.Field(i).Tag.Get("deep") == "-" {
				continue // field wants to be ignored
			}

			c.push(aType.Field(i).Name) // push field name to buff

			// Get the Value for each field, e.g. FirstName has Type = string,
			// Kind = reflect.String.
			af := a.Field(i)
			bf := b.Field(i)

			// Recurse to compare the field values
			c.equals(af, bf, level+1)

			c.pop() // pop field name from buff

			if len(c.diff) >= MaxDiff {
				break
			}
		}
	case reflect.Map:
		/*
			The variables are maps like:
				map[string]int{
					"foo": 1,
					"bar": 2,
				}
			Type = map[string]int, Kind = reflect.Map
			Or:
				type T map[string]int{}
			Type = <pkg>.T, Kind = reflect.Map
			Iterate through the map keys (foo, bar), recurse into their values.
		*/

		if a.IsNil() || b.IsNil() {
			if NilMapsAreEmpty {
				if a.IsNil() && b.Len() != 0 {
					c.saveDiff("<nil map>", b)
					return
				} else if a.Len() != 0 && b.IsNil() {
					c.saveDiff(a, "<nil map>")
					return
				}
			} else {
				if a.IsNil() && !b.IsNil() {
					c.saveDiff("<nil map>", b)
				} else if !a.IsNil() && b.IsNil() {
					c.saveDiff(a, "<nil map>")
				}
			}
			return
		}

		if a.Pointer() == b.Pointer() {
			return
		}

		for _, key := range a.MapKeys() {
			c.push(fmt.Sprintf("map[%v]", key))

			aVal := a.MapIndex(key)
			bVal := b.MapIndex(key)
			if bVal.IsValid() {
				c.equals(aVal, bVal, level+1)
			} else {
				c.saveDiff(aVal, "<does not have key>")
			}

			c.pop()

			if len(c.diff) >= MaxDiff {
				return
			}
		}

		for _, key := range b.MapKeys() {
			if aVal := a.MapIndex(key); aVal.IsValid() {
				continue
			}

			c.push(fmt.Sprintf("map[%v]", key))
			c.saveDiff("<does not have key>", b.MapIndex(key))
			c.pop()
			if len(c.diff) >= MaxDiff {
				return
			}
		}
	case reflect.Array:
		n := a.Len()
		for i := 0; i < n; i++ {
			c.push(fmt.Sprintf("array[%d]", i))
			c.equals(a.Index(i), b.Index(i), level+1)
			c.pop()
			if len(c.diff) >= MaxDiff {
				break
			}
		}
	case reflect.Slice:
		if NilSlicesAreEmpty {
			if a.IsNil() && b.Len() != 0 {
				c.saveDiff("<nil slice>", b)
				return
			} else if a.Len() != 0 && b.IsNil() {
				c.saveDiff(a, "<nil slice>")
				return
			}
		} else {
			if a.IsNil() && !b.IsNil() {
				c.saveDiff("<nil slice>", b)
				return
			} else if !a.IsNil() && b.IsNil() {
				c.saveDiff(a, "<nil slice>")
				return
			}
		}

		aLen := a.Len()
		bLen := b.Len()

		if a.Pointer() == b.Pointer() && aLen == bLen {
			return
		}

		n := aLen
		if bLen > aLen {
			n = bLen
		}
		for i := 0; i < n; i++ {
			c.push(fmt.Sprintf("slice[%d]", i))
			if i < aLen && i < bLen {
				c.equals(a.Index(i), b.Index(i), level+1)
			} else if i < aLen {
				c.saveDiff(a.Index(i), "<no value>")
			} else {
				c.saveDiff("<no value>", b.Index(i))
			}
			c.pop()
			if len(c.diff) >= MaxDiff {
				break
			}
		}

	/////////////////////////////////////////////////////////////////////
	// Primitive kinds
	/////////////////////////////////////////////////////////////////////

	case reflect.Float32, reflect.Float64:
		// Round floats to FloatPrecision decimal places to compare with
		// user-defined precision. As is commonly know, floats have "imprecision"
		// such that 0.1 becomes 0.100000001490116119384765625. This cannot
		// be avoided; it can only be handled. Issue 30 suggested that floats
		// be compared using an epsilon: equal = |a-b| < epsilon.
		// In many cases the result is the same, but I think epsilon is a little
		// less clear for users to reason about. See issue 30 for details.
		aval := fmt.Sprintf(c.floatFormat, a.Float())
		bval := fmt.Sprintf(c.floatFormat, b.Float())
		if aval != bval {
			c.saveDiff(a.Float(), b.Float())
		}
	case reflect.Bool:
		if a.Bool() != b.Bool() {
			c.saveDiff(a.Bool(), b.Bool())
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if a.Int() != b.Int() {
			c.saveDiff(a.Int(), b.Int())
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if a.Uint() != b.Uint() {
			c.saveDiff(a.Uint(), b.Uint())
		}
	case reflect.String:
		if a.String() != b.String() {
			c.saveDiff(a.String(), b.String())
		}

	default:
		logError(ErrNotHandled)
	}
}

func (c *cmp) push(name string) {
	c.buff = append(c.buff, name)
}

func (c *cmp) pop() {
	if len(c.buff) > 0 {
		c.buff = c.buff[0 : len(c.buff)-1]
	}
}

func (c *cmp) saveDiff(aval, bval interface{}) {
	if len(c.buff) > 0 {
		varName := strings.Join(c.buff, ".")
		c.diff = append(c.diff, fmt.Sprintf("%s: %v != %v", varName, aval, bval))
	} else {
		c.diff = append(c.diff, fmt.Sprintf("%v != %v", aval, bval))
	}
}

func logError(err error) {
	if LogErrors {
		log.Println(err)
	}
}
