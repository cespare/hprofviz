package main

import (
	"bufio"
	"bytes"
	"container/heap"
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"

	"github.com/dustin/go-humanize"
)

// Experiment with hprof binary format.

type reader struct {
	*bufio.Reader

	idSize  int
	scratch [8]byte

	strings       map[uint64]string
	classByID     map[uint64]*class
	classBySerial map[uint32]*class
	frameByID     map[uint64]*frame
	traceBySerial map[uint32]*trace

	total                  int64
	instanceOverhead       int64
	objectArrayOverhead    int64
	primitiveArrayOverhead int64
	traceSizes             map[uint32]int64

	tags    [256]int
	subTags [256]int
}

func newReader(r io.Reader) *reader {
	return &reader{
		Reader:        bufio.NewReader(r),
		strings:       make(map[uint64]string),
		classByID:     make(map[uint64]*class),
		classBySerial: make(map[uint32]*class),
		frameByID:     make(map[uint64]*frame),
		traceBySerial: make(map[uint32]*trace),
		traceSizes:    make(map[uint32]int64),
	}
}

// TODO: Compute/guess overhead more accurately.
// These header sizes are correct for 64-bit OpenJDK 8, empirically.
const (
	instanceHeaderSize       = 16
	objectArrayHeaderSize    = 24
	primitiveArrayHeaderSize = 24
)

type readerError struct {
	err error
}

func (r *reader) error(err error) {
	panic(readerError{err})
}

func (r *reader) errorf(format string, args ...interface{}) {
	r.error(fmt.Errorf(format, args...))
}

func (r *reader) u1() byte {
	b := r.scratch[:1]
	if _, err := io.ReadFull(r, b); err != nil {
		r.error(err)
	}
	return b[0]
}

func (r *reader) u2() uint16 {
	b := r.scratch[:2]
	if _, err := io.ReadFull(r, b); err != nil {
		r.error(err)
	}
	return binary.BigEndian.Uint16(b)
}

func (r *reader) u4() uint32 {
	b := r.scratch[:4]
	if _, err := io.ReadFull(r, b); err != nil {
		r.error(err)
	}
	return binary.BigEndian.Uint32(b)
}

func (r *reader) u8() uint64 {
	b := r.scratch[:8]
	if _, err := io.ReadFull(r, b); err != nil {
		r.error(err)
	}
	return binary.BigEndian.Uint64(b)
}

func (r *reader) id() uint64 {
	return r.u8()
}

func (r *reader) bytes(n int) []byte {
	var b []byte
	if n <= len(r.scratch) {
		b = r.scratch[:n]
	} else {
		b = make([]byte, n)
	}
	if _, err := io.ReadFull(r, b); err != nil {
		r.error(err)
	}
	return b
}

func (r *reader) ignore(n int) {
	if _, err := io.CopyN(ioutil.Discard, r, int64(n)); err != nil {
		r.error(err)
	}
}

type class struct {
	serial           uint32
	id               uint64
	stackTraceSerial uint32
	name             string
}

type frame struct {
	id         uint64
	methodName string
	methodSig  string
	filename   string
	class      *class
	lineNum    uint32
}

type trace struct {
	serial       uint32
	threadSerial uint32
	frames       []*frame
}

func (t *trace) String() string {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "trace %d\n", t.serial)
	for _, frame := range t.frames {
		fmt.Fprintf(&buf, "  %s [%s] | %s:%d\n",
			frame.methodName, frame.methodSig, frame.filename, frame.lineNum)
	}
	return buf.String()
}

func (r *reader) readString(n int) {
	id := r.id()
	s := string(r.bytes(n - r.idSize))
	r.strings[id] = s
}

func (r *reader) readClass(n int) {
	serial := r.u4()
	id := r.id()
	stackTraceSerial := r.u4()
	nameID := r.id()
	name, ok := r.strings[nameID]
	if !ok {
		r.errorf("class referred to unknown name %d", nameID)
	}
	c := &class{
		serial:           serial,
		id:               id,
		stackTraceSerial: stackTraceSerial,
		name:             name,
	}
	r.classByID[id] = c
	r.classBySerial[serial] = c
}

var unknownFile = "<unknown>"

func (r *reader) readFrame(_ int) {
	id := r.id()
	sid := r.id()
	methodName, ok := r.strings[sid]
	if !ok {
		r.errorf("frame referred to unknown method name string %d", sid)
	}
	sid = r.id()
	methodSig, ok := r.strings[sid]
	if !ok {
		r.errorf("frame referred to unknown method signature string %d", sid)
	}
	sid = r.id()
	filename := unknownFile
	if sid > 0 {
		filename, ok = r.strings[sid]
		if !ok {
			r.errorf("frame referred to unknown filename string %d", sid)
		}
	}
	serial := r.u4()
	c, ok := r.classBySerial[serial]
	if !ok {
		r.errorf("frame referred to unknown class serial %d", serial)
	}
	lineNum := r.u4()
	f := &frame{
		id:         id,
		methodName: methodName,
		methodSig:  methodSig,
		filename:   filename,
		class:      c,
		lineNum:    lineNum,
	}
	r.frameByID[id] = f
}

func (r *reader) readTrace(_ int) {
	serial := r.u4()
	threadSerial := r.u4()
	n := int(r.u4())
	frames := make([]*frame, n)
	for i := range frames {
		id := r.id()
		f, ok := r.frameByID[id]
		if !ok {
			r.errorf("trace referred to unknown frame id %d", id)
		}
		frames[i] = f
	}
	t := &trace{
		serial:       serial,
		threadSerial: threadSerial,
		frames:       frames,
	}
	r.traceBySerial[serial] = t
}

func (r *reader) basicSize(typ byte) int {
	switch typ {
	case 2: // object
		return r.idSize
	case 4: // boolean
		return 1
	case 5: // char
		return 2
	case 6: // float
		return 4
	case 7: // double
		return 8
	case 8: // byte
		return 1
	case 9: // short
		return 2
	case 10: // int
		return 4
	case 11: // long
		return 8
	default:
		r.errorf("unexpected basic type %x", typ)
	}
	return 0
}

func (r *reader) readHeapDumpSegment() int {
	tag := r.u1()
	r.subTags[tag]++
	n := 1
	switch tag {
	case 0xff: // ROOT UNKNOWN
		r.id()
		n += r.idSize
	case 0x01: // ROOT JNI GLOBAL
		r.id()
		r.id()
		n += r.idSize + r.idSize
	case 0x02: // ROOT JNI LOCAL
		r.id()
		r.u4()
		r.u4()
		n += r.idSize + 4 + 4
	case 0x03: // ROOT JAVA FRAME
		r.id()
		r.u4()
		r.u4()
		n += r.idSize + 4 + 4
	case 0x04: // ROOT NATIVE STACK
		r.id()
		r.u4()
		n += r.idSize + 4
	case 0x05: // ROOT STICKY CLASS
		r.id()
		n += r.idSize
	case 0x06: // ROOT THREAD BLOCK
		r.id()
		r.u4()
		n += r.idSize + 4
	case 0x07: // ROOT MONITOR USED
		r.id()
		n += r.idSize
	case 0x08: // ROOT THREAD OBJECT
		r.id()
		r.u4()
		r.u4()
		n += r.idSize + 4 + 4
	case 0x20: // CLASS DUMP
		classObjectID := r.id()
		c, ok := r.classByID[classObjectID]
		if !ok {
			r.errorf("class dump referred to bad class object id %d", classObjectID)
		}
		_ = c
		r.u4() // stack trace serial #
		r.id() // super class object ID
		r.id() // class loader object ID
		r.id() // signers object ID
		r.id() // protection domain object ID
		r.id() // reserved
		r.id() // reserved
		r.u4() // instance size
		n += r.idSize + 4 + r.idSize + r.idSize + r.idSize + r.idSize + r.idSize + r.idSize + 4

		numCP := int(r.u2())
		n += 2
		//ln("CP", numCP)
		for i := 0; i < numCP; i++ {
			r.u2() // constant pool index
			typ := r.u1()
			w := r.basicSize(typ)
			r.ignore(w)
			n += 2 + 1 + w
		}

		numSF := int(r.u2())
		n += 2
		//fmt.Println("SF", numSF)
		for i := 0; i < numSF; i++ {
			r.id() // static field name string ID
			typ := r.u1()
			w := r.basicSize(typ)
			r.ignore(w)
			n += r.idSize + 1 + w
		}

		numIF := int(r.u2())
		n += 2
		//fmt.Println("IF", numIF)
		for i := 0; i < numIF; i++ {
			r.id() // field name string ID
			r.u1() // type of field
			n += r.idSize + 1
		}
	case 0x21: // INSTANCE DUMP
		r.id() // object ID
		traceSerial := r.u4()
		r.id() // class object ID
		nn := int(r.u4())
		r.ignore(nn)
		n += r.idSize + 4 + r.idSize + 4 + nn

		size := int64(nn) + instanceHeaderSize
		r.total += size
		r.instanceOverhead += instanceHeaderSize
		r.traceSizes[traceSerial] += size
	case 0x22: // OBJECT ARRAY DUMP
		r.id() // array object ID
		traceSerial := r.u4()
		nn := int(r.u4())
		r.id() // array class object ID
		for i := 0; i < nn; i++ {
			r.id()
		}
		n += r.idSize + 4 + 4 + r.idSize + nn*r.idSize

		size := int64(nn*r.idSize) + objectArrayHeaderSize
		r.total += size
		r.objectArrayOverhead += objectArrayHeaderSize
		r.traceSizes[traceSerial] += size
	case 0x23: // PRIMITIVE ARRAY DUMP
		r.id() // array object ID
		traceSerial := r.u4()
		nn := int(r.u4())
		typ := r.u1()
		w := r.basicSize(typ)
		r.ignore(nn * w)
		n += r.idSize + 4 + 4 + 1 + nn*w

		size := int64(nn*w) + primitiveArrayHeaderSize
		r.total += size
		r.primitiveArrayOverhead += primitiveArrayHeaderSize
		r.traceSizes[traceSerial] += size
	default:
		r.errorf("unknown sub-tag %x", tag)
	}
	return n
}

func (r *reader) readRecord() (done bool) {
	var b [1]byte
	if _, err := io.ReadFull(r, b[:]); err != nil {
		if err == io.EOF {
			return true
		}
		r.error(err)
	}
	tag := b[0]
	r.tags[tag]++
	r.u4() // timestamp
	n := int(r.u4())

	switch tag {
	case 0x01: // STRING IN UTF8
		r.readString(n)
	case 0x02: // LOAD CLASS
		r.readClass(n)
	case 0x04: // STACK FRAME
		r.readFrame(n)
	case 0x05: // STACK TRACE
		r.readTrace(n)
	case 0x1c: // HEAP DUMP SEGMENT
		for n > 0 {
			n -= r.readHeapDumpSegment()
		}
		//return true // TODO: remove
	default:
		r.ignore(n)
	}

	return false
}

func (r *reader) readHeader() {
	s, err := r.ReadString(0)
	if err != nil {
		r.error(err)
	}
	if s != "JAVA PROFILE 1.0.2\x00" {
		r.errorf("bad header string %q", s)
	}
	idSize := r.u4()
	if idSize != 8 {
		r.errorf("only id size of 8 handled; got %d", idSize)

	}
	r.idSize = int(idSize)
	// Skip the timestamp stuff for now.
	r.u4()
	r.u4()
}

func (r *reader) readAll() (err error) {
	defer func() {
		if e := recover(); e != nil {
			re, ok := e.(readerError)
			if !ok {
				panic(e)
			}
			err = re.err
		}
	}()

	r.readHeader()
	for !r.readRecord() {
	}
	return nil
}

func top10(m map[uint32]int64) []serialSize {
	var h serialSizes
	for serial, size := range m {
		ss := serialSize{serial: serial, size: size}
		if len(h) < 10 {
			heap.Push(&h, ss)
			continue
		}
		if ss.size > h[0].size {
			h[0] = ss
			heap.Fix(&h, 0)
		}
	}
	for i := 0; i < len(h)/2; i++ {
		j := len(h) - 1 - i
		h[i], h[j] = h[j], h[i]
	}
	return []serialSize(h)
}

type serialSize struct {
	serial uint32
	size   int64
}

type serialSizes []serialSize

func (s *serialSizes) Len() int           { return len(*s) }
func (s *serialSizes) Less(i, j int) bool { return (*s)[i].size < (*s)[j].size }
func (s *serialSizes) Swap(i, j int)      { (*s)[i], (*s)[j] = (*s)[j], (*s)[i] }
func (s *serialSizes) Push(x interface{}) { *s = append(*s, x.(serialSize)) }
func (s *serialSizes) Pop() interface{} {
	n := len(*s)
	v := (*s)[n-1]
	*s = (*s)[:n-1]
	return v
}

func main() {
	log.SetFlags(0)
	if len(os.Args) != 2 {
		log.Fatalf("usage: %s FILENAME", os.Args[0])
	}
	f, err := os.Open(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	r := newReader(f)
	if err := r.readAll(); err != nil {
		log.Fatal(err)
	}
	fmt.Println(len(r.strings), "strings")
	fmt.Println(len(r.classByID), "classes")
	fmt.Println(len(r.traceBySerial), "stack traces")
	fmt.Println()
	fmt.Println("total size:", r.total)
	fmt.Println("top 10 stacks:")
	for _, ss := range top10(r.traceSizes) {
		fmt.Printf("%d\t%d\t(%s)\n", ss.serial, ss.size, humanize.Bytes(uint64(ss.size)))
		fmt.Println(r.traceBySerial[ss.serial])
	}
	fmt.Println()
	fmt.Printf("instance overhead: %d (%s)\n",
		r.instanceOverhead, humanize.Bytes(uint64(r.instanceOverhead)))
	fmt.Printf("object array overhead: %d (%s)\n",
		r.objectArrayOverhead, humanize.Bytes(uint64(r.objectArrayOverhead)))
	fmt.Printf("primitive array overhead: %d (%s)\n",
		r.primitiveArrayOverhead, humanize.Bytes(uint64(r.primitiveArrayOverhead)))
	overhead := r.instanceOverhead + r.objectArrayOverhead + r.primitiveArrayOverhead
	fmt.Printf("total overhead: %d/%d (%s / %s) %.2f%%\n",
		overhead, r.total,
		humanize.Bytes(uint64(overhead)), humanize.Bytes(uint64(r.total)),
		(float64(overhead)/float64(r.total))*100)
	fmt.Println()
	fmt.Println("tags:")
	for i, c := range r.tags {
		if c > 0 {
			fmt.Printf("%#2x\t%d\n", i, c)
		}
	}
	fmt.Println()
	fmt.Println("sub-tags:")
	for i, c := range r.subTags {
		if c > 0 {
			fmt.Printf("%#2x\t%d\n", i, c)
		}
	}
}
