package jetfuel

import (
	"encoding/binary"
	"fmt"
	"math"
)

// ── Result types ─────────────────────────────────────────────────────────────

// RootMessage is the decoded tag-0 message containing UI state.
type RootMessage struct {
	Els   []Element
	Props []Prop
	Ts    int32

	Children []uint64
	ID       *int64  // l(c): optional i64
	Extend   *uint64 // l(o): optional varint
}

// Prop is one entry in the Props array: [wire_tag, value].
type Prop struct {
	Tag   uint8
	Value interface{}
}

// Element is one UI element in RootMessage.Els.
// type=i16, props=Map<i16,varint>, children=[]varint, id=opt<i64>, extend=opt<varint>.
type Element struct {
	Type     int16
	Props    map[int16]uint64
	Children []uint64
	ID       *int64
	Extend   *uint64
}

// StringMap is the decoded form of wire tag 17: Map<string, string>.
type StringMap map[string]string

// ── Frame splitting ────────────────────────────────────────────────────────────

// SplitFrames splits a raw binary response into message payloads.
// Each frame: 4-byte uint32 LE length followed by length bytes.
func SplitFrames(data []byte) ([][]byte, error) {
	var frames [][]byte
	for len(data) > 0 {
		if len(data) < 4 {
			break
		}
		length := int(binary.LittleEndian.Uint32(data[:4]))
		data = data[4:]
		if len(data) < length {
			return nil, fmt.Errorf("jetfuel: frame truncated: need %d have %d", length, len(data))
		}
		frames = append(frames, data[:length])
		data = data[length:]
	}
	return frames, nil
}

// ── Low-level reader ──────────────────────────────────────────────────────────

type reader struct {
	data []byte
	pos  int
}

func newReader(data []byte) *reader { return &reader{data: data} }

func (r *reader) remaining() int { return len(r.data) - r.pos }

func (r *reader) u8() (uint8, error) {
	if r.pos >= len(r.data) {
		return 0, fmt.Errorf("jetfuel: u8 eof at %d", r.pos)
	}
	v := r.data[r.pos]
	r.pos++
	return v, nil
}

func (r *reader) i16() (int16, error) {
	if r.pos+2 > len(r.data) {
		return 0, fmt.Errorf("jetfuel: i16 eof at %d", r.pos)
	}
	v := int16(binary.LittleEndian.Uint16(r.data[r.pos:]))
	r.pos += 2
	return v, nil
}

func (r *reader) i32() (int32, error) {
	if r.pos+4 > len(r.data) {
		return 0, fmt.Errorf("jetfuel: i32 eof at %d", r.pos)
	}
	v := int32(binary.LittleEndian.Uint32(r.data[r.pos:]))
	r.pos += 4
	return v, nil
}

func (r *reader) i64() (int64, error) {
	if r.pos+8 > len(r.data) {
		return 0, fmt.Errorf("jetfuel: i64 eof at %d", r.pos)
	}
	v := int64(binary.LittleEndian.Uint64(r.data[r.pos:]))
	r.pos += 8
	return v, nil
}

func (r *reader) f64() (float64, error) {
	if r.pos+8 > len(r.data) {
		return 0, fmt.Errorf("jetfuel: f64 eof at %d", r.pos)
	}
	v := math.Float64frombits(binary.LittleEndian.Uint64(r.data[r.pos:]))
	r.pos += 8
	return v, nil
}

func (r *reader) bool_() (bool, error) {
	v, err := r.u8()
	return v != 0, err
}

// varint decodes a LEB128 unsigned varint.
func (r *reader) varint() (uint64, error) {
	var val uint64
	var shift uint
	for {
		b, err := r.u8()
		if err != nil {
			return 0, err
		}
		low := uint64(b & 0x7f)
		if shift < 28 {
			val += low << shift
		} else {
			val += low * (1 << shift)
		}
		shift += 7
		if b&0x80 == 0 {
			break
		}
		if shift > 63 {
			return 0, fmt.Errorf("jetfuel: varint overflow at %d", r.pos)
		}
	}
	return val, nil
}

// str decodes a varint-length-prefixed string.
// Some prop values are binary blobs; we accept any byte sequence.
func (r *reader) str() (string, error) {
	length, err := r.varint()
	if err != nil {
		return "", err
	}
	n := int(length)
	if r.pos+n > len(r.data) {
		return "", fmt.Errorf("jetfuel: str length %d eof at %d", n, r.pos)
	}
	b := r.data[r.pos : r.pos+n]
	r.pos += n
	return string(b), nil
}

// ── Top-level decoder ─────────────────────────────────────────────────────────

// DecodeMessage decodes one frame payload into a tagged message.
// Returns (*RootMessage, nil, nil) for tag 0.
// Returns (nil, ref, nil) for tag 1 (ignored for now).
func DecodeMessage(frame []byte) (*RootMessage, error) {
	r := newReader(frame)
	tag, err := r.u8()
	if err != nil {
		return nil, fmt.Errorf("jetfuel: read top tag: %w", err)
	}
	switch tag {
	case 0:
		return decodeRoot(r)
	case 1:
		// ref message — not needed for error/session extraction
		return nil, nil
	case 2:
		return nil, nil
	default:
		return nil, fmt.Errorf("jetfuel: unknown top tag %d at offset 0", tag)
	}
}

// ── Root (tag 0) decoder ──────────────────────────────────────────────────────

// decodeRoot reads els array, props array, and a ts (i32).
func decodeRoot(r *reader) (*RootMessage, error) {
	els, err := decodeElementArray(r)
	if err != nil {
		return nil, fmt.Errorf("jetfuel: els: %w", err)
	}
	props, err := decodePropArray(r)
	if err != nil {
		return nil, fmt.Errorf("jetfuel: props: %w", err)
	}
	ts, err := r.i32()
	if err != nil {
		return nil, fmt.Errorf("jetfuel: ts: %w", err)
	}
	return &RootMessage{Els: els, Props: props, Ts: ts}, nil
}

// ── Element decoder ───────────────────────────────────────────────────────────

// An element is: type (i16), props (Map<i16,varint>), children ([]varint),
// id (opt<varint>), extend (opt<varint>).

const maxCount = 100_000

func checkCount(n uint64, ctx string) error {
	if n > maxCount {
		return fmt.Errorf("jetfuel: %s count %d > %d (misaligned?)", ctx, n, maxCount)
	}
	return nil
}

func decodeElementArray(r *reader) ([]Element, error) {
	count, err := r.varint()
	if err != nil {
		return nil, err
	}
	if err := checkCount(count, "els"); err != nil {
		return nil, err
	}
	els := make([]Element, 0, count)
	for i := uint64(0); i < count; i++ {
		startPos := r.pos
		el, err := decodeElement(r)
		if err != nil {
			return nil, fmt.Errorf("element[%d] start_offset=%d: %w", i, startPos, err)
		}
		els = append(els, el)
	}
	return els, nil
}

func decodeElement(r *reader) (Element, error) {
	var el Element
	var err error

	// type: i16
	p0 := r.pos
	typ, err := r.i16()
	if err != nil {
		return el, fmt.Errorf("type@%d: %w", p0, err)
	}
	el.Type = typ

	// props: Map<i16, varint>
	p1 := r.pos
	el.Props, err = decodeI16VarMap(r)
	if err != nil {
		return el, fmt.Errorf("props@%d: %w", p1, err)
	}

	// children: []varint
	p2 := r.pos
	el.Children, err = decodeVarintArray(r)
	if err != nil {
		return el, fmt.Errorf("children@%d: %w", p2, err)
	}

	// id: u8 flag, if non-zero read i64 (8 bytes LE)
	p3 := r.pos
	idF, err := r.bool_()
	if err != nil {
		return el, fmt.Errorf("id.flag@%d: %w", p3, err)
	}
	if idF {
		v, err := r.i64()
		if err != nil {
			return el, fmt.Errorf("id.val@%d: %w", r.pos, err)
		}
		el.ID = &v
	}

	// extend: u8 flag, if non-zero read varint
	p4 := r.pos
	extF, err := r.bool_()
	if err != nil {
		return el, fmt.Errorf("extend.flag@%d: %w", p4, err)
	}
	if extF {
		v, err := r.varint()
		if err != nil {
			return el, fmt.Errorf("extend.val@%d: %w", r.pos, err)
		}
		el.Extend = &v
	}

	_ = p0
	_ = p1
	_ = p2
	_ = p3
	_ = p4
	return el, nil
}

// ── Prop decoder ──────────────────────────────────────────────────────────────

// decodePropArray reads a varint count then N prop entries.
func decodePropArray(r *reader) ([]Prop, error) {
	count, err := r.varint()
	if err != nil {
		return nil, err
	}
	props := make([]Prop, 0, count)
	for i := uint64(0); i < count; i++ {
		p, err := decodeProp(r)
		if err != nil {
			return nil, fmt.Errorf("prop[%d]: %w", i, err)
		}
		props = append(props, p)
	}
	return props, nil
}

// decodeProp reads one prop entry: a u8 tag then the dispatched value.
// Tag 17 (Map<str,str>) carries error/session info used by ParseResponse.
func decodeProp(r *reader) (Prop, error) {
	tag, err := r.u8()
	if err != nil {
		return Prop{}, err
	}
	var val interface{}
	switch tag {
	case 0, 11:
		val, err = r.str()
	case 1:
		val, err = r.i32()
	case 3:
		val, err = decodeI32ArrayArray(r)
	case 4, 14:
		val, err = r.i64()
	case 5:
		val, err = r.f64()
	case 6:
		val, err = r.bool_()
	case 7, 10, 15:
		val, err = r.varint()
	case 8, 21, 24:
		val, err = decodeVarintArray(r)
	case 12:
		val, err = decodeTaggedStringArray(r)
	case 13:
		val = nil // reads nothing
	case 16:
		val, err = decodeI16VarMap(r)
	case 17:
		val, err = decodeStringMap(r)
	case 18:
		val, err = decodeRefStruct(r)
	case 31:
		val, err = decodeDollarRef(r)
	case 19:
		val, err = decodeMDispatch(r, 0)
	case 22:
		val, err = decodeZDispatch(r, 0)
	case 25:
		val, err = decodeL25Array(r)
	case 26:
		val, err = decodeStringArray(r)
	case 27:
		val, err = decodeI32Array(r)
	case 28:
		val, err = decodeF64Array(r)
	case 29:
		val, err = decodeBoolArray(r)
	case 30:
		val, err = decodeSRecord(r)
	default:
		return Prop{}, fmt.Errorf("jetfuel: unknown prop tag %d at offset %d", tag, r.pos)
	}
	if err != nil {
		return Prop{}, fmt.Errorf("tag %d: %w", tag, err)
	}
	return Prop{Tag: tag, Value: val}, nil
}

// ── Sub-structure decoders ────────────────────────────────────────────────────

// decodeStringMap reads a varint count + N*(str key + str val).
func decodeStringMap(r *reader) (StringMap, error) {
	count, err := r.varint()
	if err != nil {
		return nil, err
	}
	if err := checkCount(count, "strMap"); err != nil {
		return nil, err
	}
	m := make(StringMap, count)
	for i := uint64(0); i < count; i++ {
		k, err := r.str()
		if err != nil {
			return nil, fmt.Errorf("key[%d]: %w", i, err)
		}
		v, err := r.str()
		if err != nil {
			return nil, fmt.Errorf("val[%d]: %w", i, err)
		}
		m[k] = v
	}
	return m, nil
}

// decodeI16VarMap reads a varint count + N*(i16 key + varint val).
func decodeI16VarMap(r *reader) (map[int16]uint64, error) {
	count, err := r.varint()
	if err != nil {
		return nil, err
	}
	if err := checkCount(count, "i16map"); err != nil {
		return nil, err
	}
	m := make(map[int16]uint64, count)
	for i := uint64(0); i < count; i++ {
		k, err := r.i16()
		if err != nil {
			return nil, fmt.Errorf("key[%d]: %w", i, err)
		}
		v, err := r.varint()
		if err != nil {
			return nil, fmt.Errorf("val[%d]: %w", i, err)
		}
		m[k] = v
	}
	return m, nil
}

// decodeVarintArray reads a varint count + N varints.
func decodeVarintArray(r *reader) ([]uint64, error) {
	count, err := r.varint()
	if err != nil {
		return nil, err
	}
	if err := checkCount(count, "varintArr"); err != nil {
		return nil, err
	}
	arr := make([]uint64, 0, count)
	for i := uint64(0); i < count; i++ {
		v, err := r.varint()
		if err != nil {
			return nil, fmt.Errorf("[%d]: %w", i, err)
		}
		arr = append(arr, v)
	}
	return arr, nil
}

// decodeStringArray reads a varint count + N strings.
func decodeStringArray(r *reader) ([]string, error) {
	count, err := r.varint()
	if err != nil {
		return nil, err
	}
	arr := make([]string, 0, count)
	for i := uint64(0); i < count; i++ {
		v, err := r.str()
		if err != nil {
			return nil, fmt.Errorf("[%d]: %w", i, err)
		}
		arr = append(arr, v)
	}
	return arr, nil
}

// decodeI32Array reads a varint count + N i32s.
func decodeI32Array(r *reader) ([]int32, error) {
	count, err := r.varint()
	if err != nil {
		return nil, err
	}
	arr := make([]int32, 0, count)
	for i := uint64(0); i < count; i++ {
		v, err := r.i32()
		if err != nil {
			return nil, fmt.Errorf("[%d]: %w", i, err)
		}
		arr = append(arr, v)
	}
	return arr, nil
}

// decodeF64Array reads a varint count + N f64s.
func decodeF64Array(r *reader) ([]float64, error) {
	count, err := r.varint()
	if err != nil {
		return nil, err
	}
	arr := make([]float64, 0, count)
	for i := uint64(0); i < count; i++ {
		v, err := r.f64()
		if err != nil {
			return nil, fmt.Errorf("[%d]: %w", i, err)
		}
		arr = append(arr, v)
	}
	return arr, nil
}

// decodeBoolArray reads a varint count + N bools.
func decodeBoolArray(r *reader) ([]bool, error) {
	count, err := r.varint()
	if err != nil {
		return nil, err
	}
	arr := make([]bool, 0, count)
	for i := uint64(0); i < count; i++ {
		v, err := r.bool_()
		if err != nil {
			return nil, fmt.Errorf("[%d]: %w", i, err)
		}
		arr = append(arr, v)
	}
	return arr, nil
}

// BRef is the decoded form of a back-reference value.
// Tags: 0:{id:i64}, 4:{id:i64,root:varint}, 1:{key:i16,root:varint},
//
//	2/3/7/8:{key:str,root:varint}, 5/6/9/10/11:{root:varint}
type BRef struct {
	Tag  uint8
	ID   *int64
	Key  interface{} // int16 or string, depending on tag
	Root *uint64
}

// decodeBRef reads one back-reference value.
func decodeBRef(r *reader) (BRef, error) {
	tag, err := r.u8()
	if err != nil {
		return BRef{}, fmt.Errorf("bref tag: %w", err)
	}
	var ref BRef
	ref.Tag = tag
	switch tag {
	case 0:
		v, err := r.i64()
		if err != nil {
			return ref, fmt.Errorf("bref[0].id: %w", err)
		}
		ref.ID = &v
	case 4:
		v, err := r.i64()
		if err != nil {
			return ref, fmt.Errorf("bref[4].id: %w", err)
		}
		ref.ID = &v
		rv, err := r.varint()
		if err != nil {
			return ref, fmt.Errorf("bref[4].root: %w", err)
		}
		ref.Root = &rv
	case 1:
		k, err := r.i16()
		if err != nil {
			return ref, fmt.Errorf("bref[1].key: %w", err)
		}
		ref.Key = k
		rv, err := r.varint()
		if err != nil {
			return ref, fmt.Errorf("bref[1].root: %w", err)
		}
		ref.Root = &rv
	case 2, 3, 7, 8:
		k, err := r.str()
		if err != nil {
			return ref, fmt.Errorf("bref[%d].key: %w", tag, err)
		}
		ref.Key = k
		rv, err := r.varint()
		if err != nil {
			return ref, fmt.Errorf("bref[%d].root: %w", tag, err)
		}
		ref.Root = &rv
	case 5, 6, 9, 10, 11:
		rv, err := r.varint()
		if err != nil {
			return ref, fmt.Errorf("bref[%d].root: %w", tag, err)
		}
		ref.Root = &rv
	default:
		return ref, fmt.Errorf("jetfuel: unknown bref tag %d at offset %d", tag, r.pos)
	}
	return ref, nil
}

// JRef is the decoded form of wire tag 18.
type JRef struct {
	Ref       BRef
	PropRef   uint64
	IsDefault bool
}

// DollarRef is the decoded form of wire tag 31.
type DollarRef struct {
	Ref BRef
}

// decodeRefStruct reads wire tag 18: {ref, prop_ref:varint, is_default:bool}.
func decodeRefStruct(r *reader) (JRef, error) {
	ref, err := decodeBRef(r)
	if err != nil {
		return JRef{}, fmt.Errorf("ref: %w", err)
	}
	pr, err := r.varint()
	if err != nil {
		return JRef{}, fmt.Errorf("prop_ref: %w", err)
	}
	isd, err := r.bool_()
	if err != nil {
		return JRef{}, fmt.Errorf("is_default: %w", err)
	}
	return JRef{Ref: ref, PropRef: pr, IsDefault: isd}, nil
}

// decodeDollarRef reads wire tag 31: {ref}.
func decodeDollarRef(r *reader) (DollarRef, error) {
	ref, err := decodeBRef(r)
	if err != nil {
		return DollarRef{}, err
	}
	return DollarRef{Ref: ref}, nil
}

// SubDispatch is a decoded sub-dispatcher result: [inner_tag, payload].
type SubDispatch struct {
	Tag   uint8
	Value interface{}
}

// ActionNode is a decoded action node: {action:str, ref:varint}.
type ActionNode struct {
	Action string
	Ref    uint64
}

// decodeSubDispatch is the entry for prop tags 19 and 22.
func decodeSubDispatch(r *reader) (SubDispatch, error) {
	return decodeMDispatch(r, 0)
}

// decodeMDispatch reads one sub-dispatcher value (wire tag 19).
// Tags: 0:sub-dispatch, 1:{ref,action,cancel}, 2:[]sub-dispatch, 3, 4, 5:{ref,type},
//
//	6:sub-dispatch, 7:{type,id}, 8:str, 9:{urls,priority}, 10:{action,ref},
//	11:{type,ref}, 12:{action,delaySeconds}, 13:{data,secret,kdt},
//	14:{text,dismissText}, 15:{ref,to}, 16:{ref,fields},
//	17:{ref,using}, 18:{ref,field}, 19:{ref,type,allowsRotation},
//	20:{ref,overlay,mode}, 21:{ref,duration,animation}
func decodeMDispatch(r *reader, depth int) (SubDispatch, error) {
	if depth > 10 {
		return SubDispatch{}, fmt.Errorf("jetfuel: M dispatch recursion depth exceeded")
	}
	tag, err := r.u8()
	if err != nil {
		return SubDispatch{}, fmt.Errorf("M tag: %w", err)
	}
	var val interface{}
	switch tag {
	case 0:
		val, err = decodeYDispatch(r)
	case 1: // {ref:varint, action:sub-dispatch, cancel:opt sub-dispatch}
		ref, e1 := r.varint()
		if e1 != nil {
			return SubDispatch{}, fmt.Errorf("M1.ref: %w", e1)
		}
		action, e2 := decodeMDispatch(r, depth+1)
		if e2 != nil {
			return SubDispatch{}, fmt.Errorf("M1.action: %w", e2)
		}
		flag, e3 := r.bool_()
		if e3 != nil {
			return SubDispatch{}, fmt.Errorf("M1.cancel.flag: %w", e3)
		}
		var cancel *SubDispatch
		if flag {
			c, e4 := decodeMDispatch(r, depth+1)
			if e4 != nil {
				return SubDispatch{}, fmt.Errorf("M1.cancel.val: %w", e4)
			}
			cancel = &c
		}
		val = struct {
			Ref    uint64
			Action SubDispatch
			Cancel *SubDispatch
		}{ref, action, cancel}
	case 2: // []sub-dispatch
		cnt, e1 := r.varint()
		if e1 != nil {
			return SubDispatch{}, fmt.Errorf("M2.count: %w", e1)
		}
		if err := checkCount(cnt, "M2.n(e)"); err != nil {
			return SubDispatch{}, err
		}
		arr := make([]SubDispatch, 0, cnt)
		for i := uint64(0); i < cnt; i++ {
			m, e := decodeMDispatch(r, depth+1)
			if e != nil {
				return SubDispatch{}, fmt.Errorf("M2[%d]: %w", i, e)
			}
			arr = append(arr, m)
		}
		val = arr
	case 3: // {url:varint, body:varint, complete:opt, error:opt, optimistic:opt}
		val, err = decodeMCase3(r, depth)
	case 4: // {action:sub-dispatch, intensity:i16}
		action, e1 := decodeMDispatch(r, depth+1)
		if e1 != nil {
			return SubDispatch{}, fmt.Errorf("M4.action: %w", e1)
		}
		intensity, e2 := r.i16()
		if e2 != nil {
			return SubDispatch{}, fmt.Errorf("M4.intensity: %w", e2)
		}
		val = struct {
			Action    SubDispatch
			Intensity int16
		}{action, intensity}
	case 5: // {ref:varint, type:u8}
		ref, e1 := r.varint()
		if e1 != nil {
			return SubDispatch{}, fmt.Errorf("M5.ref: %w", e1)
		}
		typ, e2 := r.u8()
		if e2 != nil {
			return SubDispatch{}, fmt.Errorf("M5.type: %w", e2)
		}
		val = struct {
			Ref  uint64
			Type uint8
		}{ref, typ}
	case 6:
		val, err = decodeKDispatch(r)
	case 7: // {type:u8, id:opt i64}
		typ, e1 := r.u8()
		if e1 != nil {
			return SubDispatch{}, fmt.Errorf("M7.type: %w", e1)
		}
		flag, e2 := r.bool_()
		if e2 != nil {
			return SubDispatch{}, fmt.Errorf("M7.id.flag: %w", e2)
		}
		var id *int64
		if flag {
			v, e3 := r.i64()
			if e3 != nil {
				return SubDispatch{}, fmt.Errorf("M7.id.val: %w", e3)
			}
			id = &v
		}
		val = struct {
			Type uint8
			ID   *int64
		}{typ, id}
	case 8: // str
		val, err = r.str()
	case 9: // {urls:[]str, priority:u8}
		cnt, e1 := r.varint()
		if e1 != nil {
			return SubDispatch{}, fmt.Errorf("M9.cnt: %w", e1)
		}
		if err := checkCount(cnt, "M9.urls"); err != nil {
			return SubDispatch{}, err
		}
		urls := make([]string, 0, cnt)
		for i := uint64(0); i < cnt; i++ {
			s, e := r.str()
			if e != nil {
				return SubDispatch{}, fmt.Errorf("M9.url[%d]: %w", i, e)
			}
			urls = append(urls, s)
		}
		pri, e2 := r.u8()
		if e2 != nil {
			return SubDispatch{}, fmt.Errorf("M9.priority: %w", e2)
		}
		val = struct {
			URLs     []string
			Priority uint8
		}{urls, pri}
	case 10: // {action:str, ref:varint}
		action, e1 := r.str()
		if e1 != nil {
			return SubDispatch{}, fmt.Errorf("M10.action: %w", e1)
		}
		ref, e2 := r.varint()
		if e2 != nil {
			return SubDispatch{}, fmt.Errorf("M10.ref: %w", e2)
		}
		val = ActionNode{Action: action, Ref: ref}
	case 11: // {type:u8, ref:varint}
		typ, e1 := r.u8()
		if e1 != nil {
			return SubDispatch{}, fmt.Errorf("M11.type: %w", e1)
		}
		ref, e2 := r.varint()
		if e2 != nil {
			return SubDispatch{}, fmt.Errorf("M11.ref: %w", e2)
		}
		val = struct {
			Type uint8
			Ref  uint64
		}{typ, ref}
	case 12: // {action:sub-dispatch, delaySeconds:i16}
		action, e1 := decodeMDispatch(r, depth+1)
		if e1 != nil {
			return SubDispatch{}, fmt.Errorf("M12.action: %w", e1)
		}
		delay, e2 := r.i16()
		if e2 != nil {
			return SubDispatch{}, fmt.Errorf("M12.delay: %w", e2)
		}
		val = struct {
			Action       SubDispatch
			DelaySeconds int16
		}{action, delay}
	case 13: // {data:str, secret:str, knownDeviceToken:str}
		d, e1 := r.str()
		s, e2 := r.str()
		k, e3 := r.str()
		if e1 != nil || e2 != nil || e3 != nil {
			return SubDispatch{}, fmt.Errorf("M13: %v %v %v", e1, e2, e3)
		}
		val = struct{ Data, Secret, KnownDeviceToken string }{d, s, k}
	case 14: // {text:str, dismissText:opt str}
		text, e1 := r.str()
		if e1 != nil {
			return SubDispatch{}, fmt.Errorf("M14.text: %w", e1)
		}
		flag, e2 := r.bool_()
		if e2 != nil {
			return SubDispatch{}, fmt.Errorf("M14.dismiss.flag: %w", e2)
		}
		var dismiss *string
		if flag {
			ds, e3 := r.str()
			if e3 != nil {
				return SubDispatch{}, fmt.Errorf("M14.dismiss.val: %w", e3)
			}
			dismiss = &ds
		}
		val = struct {
			Text        string
			DismissText *string
		}{text, dismiss}
	case 15: // {ref:varint, to:varint}
		ref, e1 := r.varint()
		to, e2 := r.varint()
		if e1 != nil || e2 != nil {
			return SubDispatch{}, fmt.Errorf("M15: %v %v", e1, e2)
		}
		val = struct{ Ref, To uint64 }{ref, to}
	case 16: // {ref:varint, fields:[]str}
		ref, e1 := r.varint()
		if e1 != nil {
			return SubDispatch{}, fmt.Errorf("M16.ref: %w", e1)
		}
		cnt, e2 := r.varint()
		if e2 != nil {
			return SubDispatch{}, fmt.Errorf("M16.fields.cnt: %w", e2)
		}
		if err := checkCount(cnt, "M16.fields"); err != nil {
			return SubDispatch{}, err
		}
		fields := make([]string, 0, cnt)
		for i := uint64(0); i < cnt; i++ {
			s, e := r.str()
			if e != nil {
				return SubDispatch{}, fmt.Errorf("M16.fields[%d]: %w", i, e)
			}
			fields = append(fields, s)
		}
		val = struct {
			Ref    uint64
			Fields []string
		}{ref, fields}
	case 17: // {ref:varint, using:varint}
		ref, e1 := r.varint()
		using, e2 := r.varint()
		if e1 != nil || e2 != nil {
			return SubDispatch{}, fmt.Errorf("M17: %v %v", e1, e2)
		}
		val = struct{ Ref, Using uint64 }{ref, using}
	case 18: // {ref:varint, field:str}
		ref, e1 := r.varint()
		if e1 != nil {
			return SubDispatch{}, fmt.Errorf("M18.ref: %w", e1)
		}
		field, e2 := r.str()
		if e2 != nil {
			return SubDispatch{}, fmt.Errorf("M18.field: %w", e2)
		}
		val = struct {
			Ref   uint64
			Field string
		}{ref, field}
	case 19: // {ref:varint, type:u8, allowsRotation:bool}
		ref, e1 := r.varint()
		if e1 != nil {
			return SubDispatch{}, fmt.Errorf("M19.ref: %w", e1)
		}
		typ, e2 := r.u8()
		if e2 != nil {
			return SubDispatch{}, fmt.Errorf("M19.type: %w", e2)
		}
		ar, e3 := r.bool_()
		if e3 != nil {
			return SubDispatch{}, fmt.Errorf("M19.ar: %w", e3)
		}
		val = struct {
			Ref            uint64
			Type           uint8
			AllowsRotation bool
		}{ref, typ, ar}
	case 20: // {ref:varint, overlay:varint, mode:str}
		ref, e1 := r.varint()
		ov, e2 := r.varint()
		mode, e3 := r.str()
		if e1 != nil || e2 != nil || e3 != nil {
			return SubDispatch{}, fmt.Errorf("M20")
		}
		val = struct {
			Ref, Overlay uint64
			Mode         string
		}{ref, ov, mode}
	case 21: // {ref:varint, duration:i16, animation:bool}
		ref, e1 := r.varint()
		if e1 != nil {
			return SubDispatch{}, fmt.Errorf("M21.ref: %w", e1)
		}
		dur, e2 := r.i16()
		if e2 != nil {
			return SubDispatch{}, fmt.Errorf("M21.dur: %w", e2)
		}
		anim, e3 := r.bool_()
		if e3 != nil {
			return SubDispatch{}, fmt.Errorf("M21.anim: %w", e3)
		}
		val = struct {
			Ref       uint64
			Duration  int16
			Animation bool
		}{ref, dur, anim}
	default:
		return SubDispatch{}, fmt.Errorf("jetfuel: unknown M tag %d at offset %d", tag, r.pos)
	}
	if err != nil {
		return SubDispatch{}, fmt.Errorf("M[%d]: %w", tag, err)
	}
	return SubDispatch{Tag: tag, Value: val}, nil
}

func decodeMCase3(r *reader, depth int) (interface{}, error) {
	url, e1 := r.varint()
	if e1 != nil {
		return nil, fmt.Errorf("M3.url: %w", e1)
	}
	body, e2 := r.varint()
	if e2 != nil {
		return nil, fmt.Errorf("M3.body: %w", e2)
	}
	readOptM := func(name string) (*SubDispatch, error) {
		flag, e := r.bool_()
		if e != nil {
			return nil, fmt.Errorf("%s.flag: %w", name, e)
		}
		if !flag {
			return nil, nil
		}
		m, e2 := decodeMDispatch(r, depth+1)
		if e2 != nil {
			return nil, fmt.Errorf("%s.val: %w", name, e2)
		}
		return &m, nil
	}
	complete, e3 := readOptM("M3.complete")
	if e3 != nil {
		return nil, e3
	}
	errM, e4 := readOptM("M3.error")
	if e4 != nil {
		return nil, e4
	}
	opt, e5 := readOptM("M3.optimistic")
	if e5 != nil {
		return nil, e5
	}
	_ = url
	_ = body
	_ = complete
	_ = errM
	_ = opt
	return nil, nil
}

// decodeYDispatch reads a sub-dispatcher value.
// Tags: 0:ref, 1:(ref,varint), 2:(ref,i16), 3:(ref,str), 4:(ref,varint,opt i16),
//
//	5:(ref,varint), 6:(ref,opt(varint,varint)), 7:(ref,varint), 8:(ref,varint)
func decodeYDispatch(r *reader) (SubDispatch, error) {
	tag, err := r.u8()
	if err != nil {
		return SubDispatch{}, fmt.Errorf("y tag: %w", err)
	}
	var val interface{}
	switch tag {
	case 0: // ref
		val, err = decodeBRef(r)
	case 1, 5, 7, 8: // (bref, varint)
		ref, e1 := decodeBRef(r)
		if e1 != nil {
			return SubDispatch{}, fmt.Errorf("y%d.ref: %w", tag, e1)
		}
		v, e2 := r.varint()
		if e2 != nil {
			return SubDispatch{}, fmt.Errorf("y%d.val: %w", tag, e2)
		}
		val = struct {
			Ref BRef
			Val uint64
		}{ref, v}
	case 2: // (bref, i16)
		ref, e1 := decodeBRef(r)
		if e1 != nil {
			return SubDispatch{}, fmt.Errorf("y2.ref: %w", e1)
		}
		v, e2 := r.i16()
		if e2 != nil {
			return SubDispatch{}, fmt.Errorf("y2.val: %w", e2)
		}
		val = struct {
			Ref BRef
			Val int16
		}{ref, v}
	case 3: // (bref, str)
		ref, e1 := decodeBRef(r)
		if e1 != nil {
			return SubDispatch{}, fmt.Errorf("y3.ref: %w", e1)
		}
		s, e2 := r.str()
		if e2 != nil {
			return SubDispatch{}, fmt.Errorf("y3.val: %w", e2)
		}
		val = struct {
			Ref BRef
			Str string
		}{ref, s}
	case 4: // (bref, varint, opt i16)
		ref, e1 := decodeBRef(r)
		if e1 != nil {
			return SubDispatch{}, fmt.Errorf("y4.ref: %w", e1)
		}
		v, e2 := r.varint()
		if e2 != nil {
			return SubDispatch{}, fmt.Errorf("y4.val: %w", e2)
		}
		flag, e3 := r.bool_()
		if e3 != nil {
			return SubDispatch{}, fmt.Errorf("y4.opt.flag: %w", e3)
		}
		var optI16 *int16
		if flag {
			iv, e4 := r.i16()
			if e4 != nil {
				return SubDispatch{}, fmt.Errorf("y4.opt.val: %w", e4)
			}
			optI16 = &iv
		}
		val = struct {
			Ref      BRef
			Val      uint64
			OptInt16 *int16
		}{ref, v, optI16}
	case 6: // (bref, opt (varint, varint))
		ref, e1 := decodeBRef(r)
		if e1 != nil {
			return SubDispatch{}, fmt.Errorf("y6.ref: %w", e1)
		}
		flag, e2 := r.bool_()
		if e2 != nil {
			return SubDispatch{}, fmt.Errorf("y6.opt.flag: %w", e2)
		}
		var pair *[2]uint64
		if flag {
			a, e3 := r.varint()
			if e3 != nil {
				return SubDispatch{}, fmt.Errorf("y6.a: %w", e3)
			}
			b, e4 := r.varint()
			if e4 != nil {
				return SubDispatch{}, fmt.Errorf("y6.b: %w", e4)
			}
			p := [2]uint64{a, b}
			pair = &p
		}
		val = struct {
			Ref  BRef
			Pair *[2]uint64
		}{ref, pair}
	default:
		return SubDispatch{}, fmt.Errorf("jetfuel: unknown y tag %d at offset %d", tag, r.pos)
	}
	if err != nil {
		return SubDispatch{}, fmt.Errorf("y[%d]: %w", tag, err)
	}
	return SubDispatch{Tag: tag, Value: val}, nil
}

// decodeKDispatch reads a sub-dispatcher value.
// Tags: 0,9:{url,preview?,replace}, 1:{url,body?,preview?,replace},
//
//	2,3,4,5,6: const, reads nothing, 8:{url}
func decodeKDispatch(r *reader) (SubDispatch, error) {
	tag, err := r.u8()
	if err != nil {
		return SubDispatch{}, fmt.Errorf("k tag: %w", err)
	}
	switch tag {
	case 0, 9: // {url:varint, preview:opt varint, replace:bool}
		url, _ := r.varint()
		flag, _ := r.bool_()
		if flag {
			r.varint()
		}
		rep, _ := r.bool_()
		_ = url
		_ = rep
	case 1: // {url:varint, body:opt varint, preview:opt varint, replace:bool}
		r.varint()
		f1, _ := r.bool_()
		if f1 {
			r.varint()
		}
		f2, _ := r.bool_()
		if f2 {
			r.varint()
		}
		r.bool_()
	case 2, 3, 4, 5, 6: // const, reads nothing
	case 8: // {url:varint}
		r.varint()
	default:
		return SubDispatch{}, fmt.Errorf("jetfuel: unknown k tag %d at offset %d", tag, r.pos)
	}
	return SubDispatch{Tag: tag}, nil
}

// decodeZDispatch reads one sub-dispatcher value (wire tag 22).
// Tags: 0,15:{ref}, 1-8:{ref,value:varint or []varint},
//
//	9-11:{ref,value:str}, 12,13:(self,self), 14:self
func decodeZDispatch(r *reader, depth int) (SubDispatch, error) {
	if depth > 10 {
		return SubDispatch{}, fmt.Errorf("jetfuel: z dispatch recursion depth exceeded")
	}
	tag, err := r.u8()
	if err != nil {
		return SubDispatch{}, fmt.Errorf("z tag: %w", err)
	}
	var val interface{}
	switch tag {
	case 0, 15: // {ref}
		val, err = decodeBRef(r)
	case 1, 2, 5, 6, 7, 8: // {ref, value:varint}
		ref, e1 := decodeBRef(r)
		if e1 != nil {
			return SubDispatch{}, fmt.Errorf("z%d.ref: %w", tag, e1)
		}
		v, e2 := r.varint()
		if e2 != nil {
			return SubDispatch{}, fmt.Errorf("z%d.val: %w", tag, e2)
		}
		val = struct {
			Ref BRef
			Val uint64
		}{ref, v}
	case 3, 4: // {ref, value:[]varint}
		ref, e1 := decodeBRef(r)
		if e1 != nil {
			return SubDispatch{}, fmt.Errorf("z%d.ref: %w", tag, e1)
		}
		arr, e2 := decodeVarintArray(r)
		if e2 != nil {
			return SubDispatch{}, fmt.Errorf("z%d.val: %w", tag, e2)
		}
		val = struct {
			Ref BRef
			Arr []uint64
		}{ref, arr}
	case 9, 10, 11: // {ref, value:str}
		ref, e1 := decodeBRef(r)
		if e1 != nil {
			return SubDispatch{}, fmt.Errorf("z%d.ref: %w", tag, e1)
		}
		s, e2 := r.str()
		if e2 != nil {
			return SubDispatch{}, fmt.Errorf("z%d.val: %w", tag, e2)
		}
		val = struct {
			Ref BRef
			Str string
		}{ref, s}
	case 12, 13: // (sub-dispatch, sub-dispatch)
		a, e1 := decodeZDispatch(r, depth+1)
		if e1 != nil {
			return SubDispatch{}, fmt.Errorf("z%d.a: %w", tag, e1)
		}
		b, e2 := decodeZDispatch(r, depth+1)
		if e2 != nil {
			return SubDispatch{}, fmt.Errorf("z%d.b: %w", tag, e2)
		}
		val = [2]SubDispatch{a, b}
	case 14: // sub-dispatch
		val, err = decodeZDispatch(r, depth+1)
	default:
		return SubDispatch{}, fmt.Errorf("jetfuel: unknown z tag %d at offset %d", tag, r.pos)
	}
	if err != nil {
		return SubDispatch{}, fmt.Errorf("z[%d]: %w", tag, err)
	}
	return SubDispatch{Tag: tag, Value: val}, nil
}

// SRecord is the decoded form of wire tag 30.
// (str, Map<str,str>, Map<str,str>, str, u8)
type SRecord struct {
	Field0 string
	Field1 StringMap
	Field2 StringMap
	Field3 string
	Field4 uint8
}

// decodeSRecord reads wire tag 30: (str, Map<str,str>, Map<str,str>, str, u8).
func decodeSRecord(r *reader) (SRecord, error) {
	var s SRecord
	var err error
	s.Field0, err = r.str()
	if err != nil {
		return s, fmt.Errorf("S[0]: %w", err)
	}
	s.Field1, err = decodeStringMap(r)
	if err != nil {
		return s, fmt.Errorf("S[1]: %w", err)
	}
	s.Field2, err = decodeStringMap(r)
	if err != nil {
		return s, fmt.Errorf("S[2]: %w", err)
	}
	s.Field3, err = r.str()
	if err != nil {
		return s, fmt.Errorf("S[3]: %w", err)
	}
	s.Field4, err = r.u8()
	if err != nil {
		return s, fmt.Errorf("S[4]: %w", err)
	}
	return s, nil
}

// decodeI32ArrayArray reads a [][]i32 value.
func decodeI32ArrayArray(r *reader) ([][]int32, error) {
	count, err := r.varint()
	if err != nil {
		return nil, err
	}
	if err := checkCount(count, "i32ArrArr"); err != nil {
		return nil, err
	}
	out := make([][]int32, 0, count)
	for i := uint64(0); i < count; i++ {
		inner, err := decodeI32Array(r)
		if err != nil {
			return nil, fmt.Errorf("[%d]: %w", i, err)
		}
		out = append(out, inner)
	}
	return out, nil
}

// TaggedStringEntry is one element of the tag-12 array: (u8, str, opt_str).
type TaggedStringEntry struct {
	Tag    uint8
	Text   string
	OptStr *string
}

// decodeTaggedStringArray reads an array of (u8, str, opt_str).
func decodeTaggedStringArray(r *reader) ([]TaggedStringEntry, error) {
	count, err := r.varint()
	if err != nil {
		return nil, err
	}
	if err := checkCount(count, "taggedStrArr"); err != nil {
		return nil, err
	}
	out := make([]TaggedStringEntry, 0, count)
	for i := uint64(0); i < count; i++ {
		var e TaggedStringEntry
		e.Tag, err = r.u8()
		if err != nil {
			return nil, fmt.Errorf("[%d].tag: %w", i, err)
		}
		e.Text, err = r.str()
		if err != nil {
			return nil, fmt.Errorf("[%d].text: %w", i, err)
		}
		flag, err := r.bool_()
		if err != nil {
			return nil, fmt.Errorf("[%d].optFlag: %w", i, err)
		}
		if flag {
			s, err := r.str()
			if err != nil {
				return nil, fmt.Errorf("[%d].optStr: %w", i, err)
			}
			e.OptStr = &s
		}
		out = append(out, e)
	}
	return out, nil
}

// L25Pair is one element of the tag-25 array: ([][]i32, sub-dispatch).
type L25Pair struct {
	Matrix [][]int32
	Action SubDispatch
}

// decodeL25Array reads an array of ([][]i32, sub-dispatch).
func decodeL25Array(r *reader) ([]L25Pair, error) {
	count, err := r.varint()
	if err != nil {
		return nil, err
	}
	if err := checkCount(count, "l25Arr"); err != nil {
		return nil, err
	}
	out := make([]L25Pair, 0, count)
	for i := uint64(0); i < count; i++ {
		m, err := decodeI32ArrayArray(r)
		if err != nil {
			return nil, fmt.Errorf("[%d].L: %w", i, err)
		}
		sd, err := decodeSubDispatch(r)
		if err != nil {
			return nil, fmt.Errorf("[%d].z: %w", i, err)
		}
		out = append(out, L25Pair{Matrix: m, Action: sd})
	}
	return out, nil
}

// ── Utility: extract errors + session_token ───────────────────────────────────

// Result holds the decoded begin_login response.
type Result struct {
	ErrorMessage string // non-empty = server returned error
	SessionToken string // non-empty = next step available
	NextAction   string // e.g. "/onboarding/web/actions/login_enter_password"
}

// ParseResponse decodes all frames in a raw begin_login response body
// and extracts errors, session_token, and next action URL.
func ParseResponse(body []byte) (*Result, error) {
	frames, err := SplitFrames(body)
	if err != nil {
		return nil, err
	}
	res := &Result{}
	for _, frame := range frames {
		msg, err := DecodeMessage(frame)
		if err != nil {
			return nil, err
		}
		if msg == nil {
			continue
		}
		for _, prop := range msg.Props {
			switch prop.Tag {
			case 17:
				// Tag 17: Map<str,str> — check for errors
				sm, ok := prop.Value.(StringMap)
				if !ok {
					continue
				}
				if v, ok := sm["errors"]; ok && v != "" {
					res.ErrorMessage = v
				}
				if v, ok := sm["message"]; ok && v != "" && res.ErrorMessage == "" {
					res.ErrorMessage = v
				}
				if v, ok := sm["session_token"]; ok && v != "" {
					res.SessionToken = v
				}
			case 30:
				// Tag 30: S = (action_url, body_map, ...) — primary success data.
				// Field0 = action URL, Field1 = Map<str,str> body (has session_token).
				sr, ok := prop.Value.(SRecord)
				if !ok {
					continue
				}
				if sr.Field0 != "" && res.NextAction == "" {
					res.NextAction = sr.Field0
				}
				if v, ok := sr.Field1["session_token"]; ok && v != "" {
					res.SessionToken = v
				}
				if v, ok := sr.Field1["errors"]; ok && v != "" {
					res.ErrorMessage = v
				}
				if v, ok := sr.Field1["missing_account"]; ok && v != "" {
					res.ErrorMessage = "user_not_found:" + v
				}
				if v, ok := sr.Field2["session_token"]; ok && v != "" {
					res.SessionToken = v
				}
			}
		}
	}
	return res, nil
}
