package parse

import (
	"encoding/json"
	"io"
	"log"
	"strings"
)

// TagKey represents a DICOM Tag as a string "key" used in dicom-standard. Tag (aaaa, bbbb) => aaaabbbb as a key
type TagKey string

// Attribute represents a DICOM attribute, with ability to point to "sub-attributes" that may be in a SQ
type Attribute struct {
	Tag                 string `json:"tag"`
	Name                string `json:"name"`
	Keyword             string `json:"keyword"`
	ValueRepresentation string `json:"valueRepresentation"`
	ValueMultiplicity   string `json:"valueMultiplicity"`
	Retired             bool   `json:"retired"`
	tagKey              TagKey
	SubAttributes       map[TagKey]*Attribute
}

// IsEmpty returns true if the Attribute is effectively empty
func (a *Attribute) IsEmpty() bool {
	return a.Name == "" && a.Keyword == "" &&
		a.ValueRepresentation == "" &&
		a.ValueMultiplicity == "" && a.Tag == ""
}

// TagKey returns the corresponding TagKey of an Attribute
func (a *Attribute) TagKey() TagKey {
	if a.tagKey != "" {
		return a.tagKey
	}
	a.tagKey = tagToKey(a.Tag)
	return a.tagKey
}

// AddSubAttribute adds a subattribute in the right place to an attribute "tree"
func (a *Attribute) AddSubAttribute(at *Attribute, keys []TagKey, attributeMap AttributeMap, mapping *ModuleToAttributeItem) {
	// If a's subattribute map hasn't been made yet, do so
	if a.SubAttributes == nil {
		a.SubAttributes = make(map[TagKey]*Attribute)
	}

	// If the added attribute only has one key, it is a leaf node and we can stop adding subattributes
	if len(keys) == 1 {
		if val, ok := a.SubAttributes[keys[0]]; !ok {
			a.SubAttributes[keys[0]] = at
		} else {
			log.Printf("WARN: Duplicate Attribute Path: %s", mapping.Path) // badness
			log.Printf("WARN: Existent Attribute is: %s \n", at.Tag)       // badness
			log.Printf("WARN: New Attribute is: %s \n\n", val.Tag)         // badness
		}
		return
	}

	// The added attribute needs to go deeper
	if _, ok := a.SubAttributes[keys[0]]; !ok {
		attr := attributeMap[keys[0]]
		a.SubAttributes[keys[0]] = &attr //TODO: dont use addresses, copy prob happens anyway
	}
	// Call add sub attribute on the next level
	a.SubAttributes[keys[0]].AddSubAttribute(at, keys[1:], attributeMap, mapping)

}

// AttributeMap represents a mapping from TagKey to Attribute entities
type AttributeMap map[TagKey]Attribute

// Module represents a DICOM module
type Module struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	LinkToStandard string `json:"linkToStandard"`
	Attributes     map[TagKey]*Attribute
}

// AddAttribute adds a top level attribute to a module
func (m *Module) AddAttribute(a Attribute, path string) {
	if m.Attributes == nil {
		m.Attributes = make(map[TagKey]*Attribute)
	}

	m.Attributes[a.TagKey()] = &a
}

// ModulesByID implements the sort.Sorter interface to allow []*Modules to be sorted easily.
type ModulesByID []*Module

func (m ModulesByID) Len() int           { return len(m) }
func (m ModulesByID) Swap(i, j int)      { m[i], m[j] = m[j], m[i] }
func (m ModulesByID) Less(i, j int) bool { return m[i].ID < m[j].ID }

// ModuleMap represents a map of string module identifiers to Modules
type ModuleMap map[string]Module

// ModulePointerMap represents a map of string module IDs to module pointers
type ModulePointerMap map[string]*Module

// ToPointerMap converts a ModuleMap to a ModulePointerMap
func (m ModuleMap) ToPointerMap() ModulePointerMap {
	mp := make(ModulePointerMap)
	for k, v := range m {
		module := v // allocate a new module (copy v)
		mp[k] = &module
	}
	return mp
}

// ModuleToAttributeItem represents an item that maps modules to attributes
type ModuleToAttributeItem struct {
	ModuleID string `json:"module"`
	Tag      string `json:"tag"`
	Type     string `json:"type"`
	Path     string `json:"path"`
}

// PathTagKeys returns a slice of TagKeys from the path of the ModuleToAttributeItem
func (m ModuleToAttributeItem) PathTagKeys() []TagKey {
	keys := strings.Split(m.Path, ":")[1:]
	tagKeys := make([]TagKey, len(keys))
	// Cast strings to TagKeys
	for i := range keys {
		tagKeys[i] = TagKey(keys[i])
	}
	return tagKeys
}

// Parse consumes data from innolitics' dicom-standard format and returns a slice of parsed Modules.
func Parse(attributeSource, moduleSource, moduleToAttributesSource io.Reader) ([]*Module, error) {
	// Parse AttributeMap
	dec := json.NewDecoder(attributeSource)
	var attrMap AttributeMap
	err := dec.Decode(&attrMap)
	if err != nil {
		return []*Module{}, err
	}

	// Parse ModuleMap
	dec = json.NewDecoder(moduleSource)
	var mMap ModuleMap
	err = dec.Decode(&mMap)
	if err != nil {
		return []*Module{}, err
	}
	moduleMap := mMap.ToPointerMap()

	// Parse ModuleToAttributeItem mappings
	dec = json.NewDecoder(moduleToAttributesSource)
	var moduleToAttributes []ModuleToAttributeItem
	err = dec.Decode(&moduleToAttributes)
	if err != nil {
		return []*Module{}, err
	}

	// Associate all modules with their sets of attributes
	for _, mapping := range moduleToAttributes {
		tagKey := tagToKey(mapping.Tag)
		attr, ok := attrMap[tagKey]
		if !ok {
			log.Printf("ERROR: NO Attribute MAPPING FOUND: %s", tagKey) // badness
		}
		module := moduleMap[mapping.ModuleID]

		// Add the attribute to the module or corresponding module attribute chain:
		pathTagKeys := mapping.PathTagKeys()

		if len(pathTagKeys) == 1 {
			// Add as a top level module attribute
			module.AddAttribute(attr, mapping.Path)
		} else {
			// This is not a top level module, leave the module struct values blank but create an attribute map so subattributes can be added
			if module.Attributes == nil {
				module.Attributes = make(map[TagKey]*Attribute)
			}

			module.Attributes[pathTagKeys[0]].AddSubAttribute(&attr, pathTagKeys[1:], attrMap, &mapping) //TODO: no need for attr pointer, it's prob copied anyway since map values are not addressable
		}
	}

	// Create slice of modules to return
	i := 0
	modules := make([]*Module, len(moduleMap))
	for _, v := range moduleMap {
		modules[i] = v
		i++
	}

	return modules, nil
}

// Useful functions
func tagToKey(tag string) TagKey {
	s := strings.Replace(tag, ",", "", -1)
	s2 := strings.Replace(s, "(", "", -1)
	s3 := strings.Replace(s2, ")", "", -1)
	s4 := strings.ToLower(s3)

	return TagKey(s4)
}
