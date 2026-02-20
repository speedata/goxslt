package goxslt

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/speedata/goxml"
	"github.com/speedata/goxpath"
)

// ItemType identifies the expected type of an item in a SequenceType declaration.
type ItemType int

const (
	ItemTypeItem    ItemType = iota // item()
	ItemTypeNode                    // node()
	ItemTypeElement                 // element()
	ItemTypeString                  // xs:string
	ItemTypeInteger                 // xs:integer
	ItemTypeDouble                  // xs:double
	ItemTypeBoolean                 // xs:boolean
	ItemTypeMap                     // map(*)
	ItemTypeArray                   // array(*)
)

// Cardinality specifies how many items are expected.
type Cardinality int

const (
	CardOne        Cardinality = iota // exactly 1
	CardZeroOrOne                     // ?
	CardZeroOrMore                    // *
	CardOneOrMore                     // +
)

// SequenceType represents a parsed XSLT/XPath sequence type declaration
// such as "xs:integer", "node()*", or "xs:string?".
type SequenceType struct {
	Item        ItemType
	Cardinality Cardinality
}

// parseSequenceType parses a sequence type string like "xs:integer",
// "node()*", or "xs:string?" into a SequenceType.
func parseSequenceType(s string) (*SequenceType, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("empty sequence type")
	}

	st := &SequenceType{}

	// Check for trailing occurrence indicator.
	last := s[len(s)-1]
	switch last {
	case '?':
		st.Cardinality = CardZeroOrOne
		s = s[:len(s)-1]
	case '*':
		st.Cardinality = CardZeroOrMore
		s = s[:len(s)-1]
	case '+':
		st.Cardinality = CardOneOrMore
		s = s[:len(s)-1]
	default:
		st.Cardinality = CardOne
	}

	switch s {
	case "item()":
		st.Item = ItemTypeItem
	case "node()":
		st.Item = ItemTypeNode
	case "element()":
		st.Item = ItemTypeElement
	case "xs:string":
		st.Item = ItemTypeString
	case "xs:integer":
		st.Item = ItemTypeInteger
	case "xs:double":
		st.Item = ItemTypeDouble
	case "xs:boolean":
		st.Item = ItemTypeBoolean
	default:
		if strings.HasPrefix(s, "map(") && strings.HasSuffix(s, ")") {
			st.Item = ItemTypeMap
		} else if strings.HasPrefix(s, "array(") && strings.HasSuffix(s, ")") {
			st.Item = ItemTypeArray
		} else {
			return nil, fmt.Errorf("unsupported sequence type: %s", s)
		}
	}

	return st, nil
}

// coerceSequence validates and coerces a sequence to match the declared type.
// It returns the coerced sequence or an error if the value cannot be converted.
func coerceSequence(st *SequenceType, seq goxpath.Sequence) (goxpath.Sequence, error) {
	// Check cardinality.
	n := len(seq)
	switch st.Cardinality {
	case CardOne:
		if n != 1 {
			return nil, fmt.Errorf("expected exactly 1 item, got %d", n)
		}
	case CardZeroOrOne:
		if n > 1 {
			return nil, fmt.Errorf("expected 0 or 1 items, got %d", n)
		}
	case CardOneOrMore:
		if n == 0 {
			return nil, fmt.Errorf("expected at least 1 item, got 0")
		}
	case CardZeroOrMore:
		// any count is fine
	}

	if n == 0 {
		return seq, nil
	}

	// Coerce each item.
	result := make(goxpath.Sequence, n)
	for i, item := range seq {
		coerced, err := coerceItem(st.Item, item)
		if err != nil {
			return nil, err
		}
		result[i] = coerced
	}
	return result, nil
}

func coerceItem(typ ItemType, item any) (any, error) {
	switch typ {
	case ItemTypeItem:
		return item, nil

	case ItemTypeNode:
		if _, ok := item.(goxml.XMLNode); ok {
			return item, nil
		}
		return nil, fmt.Errorf("expected node(), got %T", item)

	case ItemTypeElement:
		if _, ok := item.(*goxml.Element); ok {
			return item, nil
		}
		return nil, fmt.Errorf("expected element(), got %T", item)

	case ItemTypeString:
		return coerceString(item)

	case ItemTypeInteger:
		return coerceInteger(item)

	case ItemTypeDouble:
		return coerceDouble(item)

	case ItemTypeBoolean:
		return coerceBoolean(item)

	case ItemTypeMap:
		if _, ok := item.(*goxpath.XPathMap); ok {
			return item, nil
		}
		return nil, fmt.Errorf("expected map(*), got %T", item)

	case ItemTypeArray:
		if _, ok := item.(*goxpath.XPathArray); ok {
			return item, nil
		}
		return nil, fmt.Errorf("expected array(*), got %T", item)

	default:
		return nil, fmt.Errorf("unknown item type %d", typ)
	}
}

func coerceString(item any) (any, error) {
	switch v := item.(type) {
	case string:
		return v, nil
	case float64:
		if v == math.Trunc(v) && !math.IsInf(v, 0) && !math.IsNaN(v) {
			return strconv.FormatInt(int64(v), 10), nil
		}
		return strconv.FormatFloat(v, 'g', -1, 64), nil
	case int:
		return strconv.Itoa(v), nil
	case bool:
		if v {
			return "true", nil
		}
		return "false", nil
	case goxml.XMLNode:
		sv, err := goxpath.StringValue(goxpath.Sequence{item})
		if err != nil {
			return nil, err
		}
		return sv, nil
	default:
		return fmt.Sprintf("%v", item), nil
	}
}

func coerceInteger(item any) (any, error) {
	switch v := item.(type) {
	case int:
		return v, nil
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, fmt.Errorf("cannot convert %v to xs:integer", v)
		}
		return int(v), nil
	case string:
		return parseStringAsInteger(v)
	case bool:
		if v {
			return 1, nil
		}
		return 0, nil
	case goxml.CharData:
		return parseStringAsInteger(v.Contents)
	default:
		return nil, fmt.Errorf("cannot convert %T to xs:integer", item)
	}
}

func parseStringAsInteger(s string) (any, error) {
	s = strings.TrimSpace(s)
	if i, err := strconv.Atoi(s); err == nil {
		return i, nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil, fmt.Errorf("cannot convert %q to xs:integer", s)
	}
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return nil, fmt.Errorf("cannot convert %q to xs:integer", s)
	}
	return int(f), nil
}

func coerceDouble(item any) (any, error) {
	switch v := item.(type) {
	case float64:
		return v, nil
	case int:
		return float64(v), nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			return nil, fmt.Errorf("cannot convert %q to xs:double", v)
		}
		return f, nil
	case bool:
		if v {
			return 1.0, nil
		}
		return 0.0, nil
	case goxml.CharData:
		f, err := strconv.ParseFloat(strings.TrimSpace(v.Contents), 64)
		if err != nil {
			return nil, fmt.Errorf("cannot convert %q to xs:double", v.Contents)
		}
		return f, nil
	default:
		return nil, fmt.Errorf("cannot convert %T to xs:double", item)
	}
}

func coerceBoolean(item any) (any, error) {
	// Handle CharData by extracting text content first.
	if cd, ok := item.(goxml.CharData); ok {
		switch cd.Contents {
		case "true", "1":
			return true, nil
		case "false", "0":
			return false, nil
		default:
			return nil, fmt.Errorf("cannot convert %q to xs:boolean", cd.Contents)
		}
	}
	// Use goxpath.BooleanValue for single-item sequences.
	b, err := goxpath.BooleanValue(goxpath.Sequence{item})
	if err != nil {
		return nil, fmt.Errorf("cannot convert %T to xs:boolean", item)
	}
	return b, nil
}
