package gen

import (
	"bytes"
	"fmt"
	"io"
	"log"

	"strings"

	"regexp"

	"github.com/gradienthealth/dicom-protos/parse"
)

// VRToProto maps a DICOM VR to the corresponding proto type (Part 5 6.2)
var VRToProto = map[string]string{
	"UT":       "string",
	"US":       "uint32", // maps 16 bit to 32 bit uint
	"UN":       "bytes",
	"UL":       "uint32",
	"UI":       "string", //TODO(suyash) create a UID proto message?
	"TM":       "int64",
	"ST":       "string",
	"SS":       "int32", // 16 bit -> 32 bit
	"SQ":       "bytes", //TODO(suyash) figure out how to map sequences best. Just bytes fallback now
	"SL":       "int32",
	"SH":       "string",
	"PN":       "string",
	"OW":       "bytes",  // 16 bit words needs to be specified somehow
	"OF":       "string", //TODO(suyash) think about just parsing as float32
	"OD":       "string",
	"OB":       "string",
	"LT":       "string",
	"LO":       "string",
	"IS":       "int32",
	"FD":       "double",
	"FL":       "float",
	"DT":       "int64", //TODO(suyash) consider using protobuf time lib
	"DS":       "float",
	"DA":       "string",
	"CS":       "string",
	"AT":       "string", // attribute tag, consider handing specially
	"AS":       "string",
	"AE":       "string",
	"UC":       "string",
	"UR":       "string", // consider url types later
	"":         "bytes",  // assuming this is unknown VR, default to bytes
	"OL":       "int32",
	"US or SS": "int64",
	"US or OW": "bytes",
	"OB or OW": "bytes",
}

const ProtoHeader = "syntax = \"proto3\";"

func AttributeProto(a *parse.Attribute) string {

	b := bytes.NewBufferString("")

	// Determine name of proto message:
	messageName := a.Keyword
	if a.Keyword == "" {
		messageName = string(a.TagKey())
		log.Printf("Blank keyword, assigning tag. Attribute: %+v", a)
	}

	fmt.Fprintf(b, "// DICOM Tag: %s\n", a.Tag)
	fmt.Fprintf(b, "message %s {\n", messageName)

	if a.ValueMultiplicity != "1" {
		fmt.Fprint(b, "\trepeated ")
	} else {
		fmt.Fprint(b, "\t")
	}

	var fieldName string
	if val, ok := VRToProto[a.ValueRepresentation]; ok {
		fieldName = val
	} else {
		log.Printf("Issue mapping VR. Offending VR: %s", a.ValueRepresentation)
	}
	fmt.Fprintf(b, "%s value = 1;\n", fieldName)

	// Add word size for OW VRs. Clients can choose to fill this in, should be 16bits
	if a.ValueRepresentation == "OW" {
		fmt.Fprint(b, "\tint32 word_size = 2;\n")
	}

	if a.ValueRepresentation == "SQ" {
		fmt.Fprint(b, "\t// This attribute has a SQ VR, no proto mapping yet\n")
	}

	fmt.Fprint(b, "}")
	return b.String()
}

// ModuleProto generates the corresponding protocol buffer for the module and writes it to w
func ModuleProto(module *parse.Module, w io.Writer) error {
	fmt.Fprintf(w, "// %s\n// Link to standard: %s\n", module.Name, module.LinkToStandard)
	fmt.Fprintln(w, ProtoHeader)
	fmt.Fprintln(w, "")

	// Write attribute messages
	for _, a := range module.Attributes {
		if a.Retired || a.IsEmpty() {
			continue // Skip
		}
		ap := AttributeProto(a)
		fmt.Fprintf(w, ap)
		fmt.Fprintln(w, "")
	}

	// Write module proto
	moduleName := strings.Replace(module.Name, " ", "", -1)
	fmt.Fprintf(w, "message %sModule {\n", moduleName)

	i := 1
	for _, a := range module.Attributes {
		if a.Retired || a.IsEmpty() {
			continue
		}
		fmt.Fprintf(w, "\t%s %s = %d;\n", a.Keyword, getFieldName(a.Name), i)
		i++
	}

	fmt.Fprintln(w, "}")

	return nil
}

func getFieldName2(name string) string {
	re := regexp.MustCompile("[A-Z][^A-Z]*")
	idxs := re.FindAllStringSubmatchIndex(name, -1)
	s := ""
	for _, idxArray := range idxs[0 : len(idxs)-1] {
		s += name[idxArray[0]:idxArray[1]]
		s += "_"
	}
	s += name[idxs[len(idxs)-1][0]:idxs[len(idxs)-1][1]]
	return strings.ToLower(s)
}

func getFieldName(name string) string {
	s := strings.ToLower(name)
	s2 := strings.Replace(s, " ", "_", -1)
	s3 := strings.Replace(s2, "-", "_", -1)
	s4 := strings.Replace(s3, "(", "", -1)
	s5 := strings.Replace(s4, ")", "", -1)
	s6 := strings.Replace(s5, "/", "_", -1)

	// Only allow ascii characters into the FieldName
	re := regexp.MustCompile("[[:^ascii:]]")
	s7 := re.ReplaceAllLiteralString(s6, "")
	return s7
}
