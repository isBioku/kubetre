// Package schema validates template parameters and values against a template's JSON Schema.
package schema

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// ErrNotObject means the document is not a JSON object.
var ErrNotObject = errors.New("must be a JSON object")

// Validate checks that doc is a JSON object satisfying the optional schema. Remote and file
// references are disabled so a template's schema can never make the server fetch anything.
func Validate(id string, schema []byte, doc json.RawMessage) error {
	var obj map[string]any
	if err := json.Unmarshal(doc, &obj); err != nil || obj == nil {
		return ErrNotObject
	}
	if len(bytes.TrimSpace(schema)) == 0 {
		return nil
	}
	schemaDoc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schema))
	if err != nil {
		return fmt.Errorf("template schema is not valid JSON: %w", err)
	}
	c := jsonschema.NewCompiler()
	c.UseLoader(jsonschema.SchemeURLLoader{})
	url := "kubetre://templates/" + id + ".json"
	if err := c.AddResource(url, schemaDoc); err != nil {
		return fmt.Errorf("template schema: %w", err)
	}
	sch, err := c.Compile(url)
	if err != nil {
		return fmt.Errorf("template schema does not compile: %w", err)
	}
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(doc))
	if err != nil {
		return errors.New("must be valid JSON")
	}
	return sch.Validate(inst)
}
