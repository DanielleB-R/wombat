// Package spectest loads the official WebAssembly spec tests, converted to JSON
// by wast2json (see `make spec-convert`), so that each milestone can run the
// commands it understands.
//
// The loader knows nothing about the rest of wombat. A milestone's test file
// walks the commands, runs the ones it can (decoding needs only `module` and
// the assert_* commands that carry a binary module), and skips the rest.
package spectest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// DefaultRoot is where `make spec-convert` puts the converted tests, relative
// to a package directory under internal/.
var DefaultRoot = filepath.Join("..", "..", "testdata", "spec", "json")

// Command is one entry of a wast2json command list. Only the fields the
// harness needs are decoded; actions and expected values are left for the
// milestones that execute modules.
type Command struct {
	// Type is "module", "register", "action", "assert_return", "assert_trap",
	// "assert_exhaustion", "assert_malformed", "assert_invalid",
	// "assert_unlinkable" or "assert_uninstantiable".
	Type string `json:"type"`
	Line int    `json:"line"`
	// Name is the module's $name, if it has one.
	Name string `json:"name,omitempty"`
	// Filename is the module file, relative to the JSON file. It is set for
	// module and for the assert_* commands that carry a module.
	Filename string `json:"filename,omitempty"`
	// Text is the failure message the spec expects ("unexpected end", ...).
	Text string `json:"text,omitempty"`
	// ModuleType is "binary" or "text" for assert_malformed. Text modules are
	// source that wast2json could not convert, so there is no .wasm for them.
	ModuleType string `json:"module_type,omitempty"`
}

// File is one converted .wast file.
type File struct {
	// Name is the test's base name, e.g. "binary" for binary.wast.
	Name string
	// Dir is the directory holding the JSON and the .wasm files.
	Dir string
	// SourceFilename is the original .wast path as recorded by wast2json.
	SourceFilename string    `json:"source_filename"`
	Commands       []Command `json:"commands"`
}

// Load reads one converted test from dir/<name>.json.
func Load(dir string) (*File, error) {
	name := filepath.Base(dir)
	data, err := os.ReadFile(filepath.Join(dir, name+".json"))
	if err != nil {
		return nil, err
	}
	var f File
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	f.Name = name
	f.Dir = dir
	return &f, nil
}

// LoadAll loads every converted test under root, sorted by name. Tests that
// wast2json could not convert (no JSON file) are returned in missing.
func LoadAll(root string) (files []*File, missing []string, err error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, nil, err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		f, err := Load(filepath.Join(root, e.Name()))
		if os.IsNotExist(err) {
			missing = append(missing, e.Name())
			continue
		}
		if err != nil {
			return nil, nil, err
		}
		files = append(files, f)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })
	sort.Strings(missing)
	return files, missing, nil
}

// ReadModule returns the bytes of the module file a command refers to.
func (f *File) ReadModule(c Command) ([]byte, error) {
	if c.Filename == "" {
		return nil, fmt.Errorf("%s:%d: %s has no module file", f.Name, c.Line, c.Type)
	}
	return os.ReadFile(filepath.Join(f.Dir, c.Filename))
}
