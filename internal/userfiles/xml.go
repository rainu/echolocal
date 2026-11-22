package userfiles

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
)

// encoding/xml accepts undeclared prefixes and xmlns:p="". DAV requires
// namespace-well-formed XML, so validate bindings explicitly before decoding.
func validateDAVXML(b []byte) error {
	d := xml.NewDecoder(bytes.NewReader(b))
	stack := []map[string]string{{"xml": "http://www.w3.org/XML/1998/namespace"}}
	for {
		t, e := d.RawToken()
		if e == io.EOF {
			return nil
		}
		if e != nil {
			return e
		}
		switch t := t.(type) {
		case xml.StartElement:
			ns := map[string]string{}
			for k, v := range stack[len(stack)-1] {
				ns[k] = v
			}
			for _, a := range t.Attr {
				if a.Name.Space == "xmlns" {
					if a.Value == "" || a.Name.Local == "xmlns" || (a.Name.Local == "xml" && a.Value != "http://www.w3.org/XML/1998/namespace") {
						return errors.New("invalid namespace")
					}
					ns[a.Name.Local] = a.Value
				} else if a.Name.Space == "" && a.Name.Local == "xmlns" {
					ns[""] = a.Value
				}
			}
			if t.Name.Space != "" && ns[t.Name.Space] == "" {
				return errors.New("unbound element prefix")
			}
			for _, a := range t.Attr {
				if a.Name.Space != "" && a.Name.Space != "xmlns" && ns[a.Name.Space] == "" {
					return errors.New("unbound attribute prefix")
				}
			}
			stack = append(stack, ns)
		case xml.EndElement:
			if len(stack) <= 1 {
				return errors.New("unexpected end")
			}
			stack = stack[:len(stack)-1]
		}
	}
}

// Adapt the standard handler's exclusive-only LOCK parser/response without
// changing owner XML or property XML. The common lock table retains real scope.
func lockScopeXML(b []byte, from, to string) ([]byte, bool, error) {
	d := xml.NewDecoder(bytes.NewReader(b))
	var out bytes.Buffer
	enc := xml.NewEncoder(&out)
	scope := 0
	changed := false
	for {
		t, e := d.Token()
		if e == io.EOF {
			break
		}
		if e != nil {
			return nil, false, e
		}
		switch v := t.(type) {
		case xml.StartElement:
			if v.Name.Space == "DAV:" && v.Name.Local == "lockscope" {
				scope++
			}
			if scope > 0 && v.Name.Space == "DAV:" && v.Name.Local == from {
				v.Name.Local = to
				changed = true
			}
			t = v
		case xml.EndElement:
			if scope > 0 && v.Name.Space == "DAV:" && v.Name.Local == from {
				v.Name.Local = to
			}
			if v.Name.Space == "DAV:" && v.Name.Local == "lockscope" {
				scope--
			}
			t = v
		}
		if e = enc.EncodeToken(t); e != nil {
			return nil, false, e
		}
	}
	if e := enc.Flush(); e != nil {
		return nil, false, e
	}
	return out.Bytes(), changed, nil
}

// Preserve arbitrary dead-property XML byte-for-byte. Only the standard live
// supportedlock property's content is extended; no synthetic duplicate property
// is inserted into the handler's live/dead property inventory.
func patchSupportedLocks(b []byte) []byte {
	d := xml.NewDecoder(bytes.NewReader(b))
	var stack []xml.Name
	start := -1
	level := 0
	copied := 0
	var out bytes.Buffer
	for {
		before := int(d.InputOffset())
		t, e := d.Token()
		if e != nil {
			break
		}
		switch v := t.(type) {
		case xml.StartElement:
			if v.Name == (xml.Name{Space: "DAV:", Local: "supportedlock"}) && len(stack) >= 2 && stack[len(stack)-1] == (xml.Name{Space: "DAV:", Local: "prop"}) && stack[len(stack)-2] == (xml.Name{Space: "DAV:", Local: "propstat"}) {
				start = before
				level = len(stack) + 1
			}
			stack = append(stack, v.Name)
		case xml.EndElement:
			if start >= 0 && len(stack) == level {
				out.Write(b[copied:start])
				out.WriteString(`<D:supportedlock xmlns:D="DAV:"><D:lockentry><D:lockscope><D:exclusive/></D:lockscope><D:locktype><D:write/></D:locktype></D:lockentry><D:lockentry><D:lockscope><D:shared/></D:lockscope><D:locktype><D:write/></D:locktype></D:lockentry></D:supportedlock>`)
				copied = int(d.InputOffset())
				start = -1
			}
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	if copied == 0 {
		return b
	}
	out.Write(b[copied:])
	return out.Bytes()
}
