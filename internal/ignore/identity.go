package ignore

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"math"
)

// identityHeader is the domain-separation prefix of a policy digest.
// The canonical body follows it with no extra separator. The header is
// Nodex-specific, so this digest cannot be confused with a hash of source
// bytes or with a policy digest from another system.
const identityHeader = "nodex.ignore.policy\n"

const (
	flagNegated       byte = 0x01
	flagDirectoryOnly byte = 0x02

	segmentDoubleStar byte = 0x01
	segmentGlob       byte = 0x02

	partStar    byte = 0x01
	partAny     byte = 0x02
	partLiteral byte = 0x03
	partClass   byte = 0x04
)

// Identity returns the digest of the compiled policy.
//
// The form is "sha256:" and 64 lowercase hexadecimal digits. SHA-256 covers
// identityHeader and a canonical body. The body lists the preset identifiers
// in lexical order and then the compiled rules, in policy order, duplicates
// included. JSON whitespace, object-field order, preset order, and spellings
// that compile to the same rule do not change the digest. Adding, removing,
// or replacing a preset identifier does. Negation, the directory-only flag,
// segment structure, literal bytes, and character-class members and their
// order do.
//
// The body is a tagged, length-prefixed tree. Every integer is an unsigned
// 32-bit big-endian value:
//
//	body    = u32(preset_count) preset* u32(rule_count) rule*
//	preset  = u32(byte_length) bytes
//	rule    = u8(flags) u32(segment_count) segment*
//	flags   = 0x01 negated | 0x02 directory-only; every other bit is zero
//	segment = 0x01                                 double-star, no payload
//	        | 0x02 u32(part_count) part*           glob
//	part    = 0x01                                 star, already collapsed
//	        | 0x02                                 one rune
//	        | 0x03 u32(byte_length) bytes          literal bytes
//	        | 0x04 class
//	class   = u8(negation) u32(item_count) (u32(lo) u32(hi))*
//
// Anchoring is the segment list produced by the compiler: the leading
// double-star of an unanchored pattern, and the extra single-element star
// after a trailing double-star. There is no separate anchor bit. The empty
// policy, including the zero Policy, has preset count zero and rule count
// zero: 00 00 00 00 00 00 00 00.
//
// Identity is a pure function of the compiled rules and is safe for
// concurrent callers.
func (p Policy) Identity() string {
	body, ok := p.canonicalBody()
	if !ok {
		// The compiler does not emit a shape this encoding cannot name.
		panic("ignore: compiled policy has no canonical encoding")
	}
	digest := sha256.New()
	_, _ = digest.Write([]byte(identityHeader))
	_, _ = digest.Write(body)
	return "sha256:" + hex.EncodeToString(digest.Sum(nil))
}

// canonicalBody encodes the compiled rules.
// ok is false only for a count that does not fit in 32 bits or for a part
// shape the compiler does not produce.
func (p Policy) canonicalBody() ([]byte, bool) {
	var enc bodyEncoder
	enc.count(len(p.presets))
	for _, id := range p.presets {
		enc.count(len(id))
		enc.buf = append(enc.buf, id...)
	}
	enc.count(len(p.rules))
	for _, rule := range p.rules {
		var flags byte
		if rule.negate {
			flags |= flagNegated
		}
		if rule.dirOnly {
			flags |= flagDirectoryOnly
		}
		enc.buf = append(enc.buf, flags)
		enc.count(len(rule.segs))
		for _, seg := range rule.segs {
			if seg.doubleStar {
				if len(seg.parts) != 0 {
					return nil, false
				}
				enc.buf = append(enc.buf, segmentDoubleStar)
				continue
			}
			enc.buf = append(enc.buf, segmentGlob)
			enc.count(len(seg.parts))
			for _, part := range seg.parts {
				if !enc.part(part) {
					return nil, false
				}
			}
		}
	}
	if enc.overflow {
		return nil, false
	}
	return enc.buf, true
}

// bodyEncoder appends the canonical body and records a count that overflowed.
type bodyEncoder struct {
	buf      []byte
	overflow bool
}

func (e *bodyEncoder) count(n int) {
	if n < 0 || uint64(n) > math.MaxUint32 {
		e.overflow = true
		return
	}
	e.buf = binary.BigEndian.AppendUint32(e.buf, uint32(n))
}

func (e *bodyEncoder) codePoint(r rune) {
	if r < 0 {
		e.overflow = true
		return
	}
	e.buf = binary.BigEndian.AppendUint32(e.buf, uint32(r))
}

// part encodes one glob part. The part must be exactly one kind.
func (e *bodyEncoder) part(part globPart) bool {
	kinds := 0
	if part.star {
		kinds++
	}
	if part.any {
		kinds++
	}
	if part.lit != "" {
		kinds++
	}
	if part.class != nil {
		kinds++
	}
	if kinds != 1 {
		return false
	}
	switch {
	case part.star:
		e.buf = append(e.buf, partStar)
	case part.any:
		e.buf = append(e.buf, partAny)
	case part.lit != "":
		e.buf = append(e.buf, partLiteral)
		e.count(len(part.lit))
		e.buf = append(e.buf, part.lit...)
	default:
		e.buf = append(e.buf, partClass)
		if part.class.negate {
			e.buf = append(e.buf, 0x01)
		} else {
			e.buf = append(e.buf, 0x00)
		}
		e.count(len(part.class.items))
		for _, item := range part.class.items {
			e.codePoint(item.lo)
			e.codePoint(item.hi)
		}
	}
	return true
}
