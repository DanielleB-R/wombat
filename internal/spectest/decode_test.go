package spectest_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DanielleB-R/wombat/decode"
	"github.com/DanielleB-R/wombat/internal/spectest"
)

// decodeModule is the one place that names the decoder's entry points.
func decodeModule(b []byte) error {
	sections, err := decode.ReadSections(bytes.NewReader(b))
	if err != nil {
		return err
	}
	_, err = decode.ParseSections(sections)
	return err
}

// knownFailures are spec commands the decoder is known to get wrong, keyed by
// "<file>:<line>", each with the reason. The test fails if a listed command
// starts passing, so entries cannot go stale. Keep this list short and
// explained; it is a to-do list, not a place to hide failures.
var knownFailures = map[string]string{}

// Milestone 1 runs the commands that need only the decoder:
//
//   - module: the binary must decode.
//   - assert_malformed (binary): the module must fail to decode.
//   - assert_invalid, assert_unlinkable, assert_uninstantiable: these modules
//     are well-formed and only fail later (validation, linking, start
//     function), so they must decode. That makes a few thousand extra
//     positive cases for free.
//
// assert_malformed with a text module needs a WAT parser (a stretch goal) and
// is skipped, as are the execution commands (assert_return and friends).
func TestSpecDecode(t *testing.T) {
	root := spectest.DefaultRoot
	if _, err := os.Stat(root); errors.Is(err, os.ErrNotExist) {
		t.Skip("spec tests not converted; run `make spec-convert` (needs git and WABT's wast2json)")
	}
	files, missing, err := spectest.LoadAll(root)
	require.NoError(t, err)
	require.NotEmpty(t, files)
	if len(missing) > 0 {
		t.Logf("wast2json could not convert: %v", missing)
	}

	var total struct{ ran, skipped, known int }
	stale := map[string]bool{}
	for key := range knownFailures {
		stale[key] = true
	}

	for _, f := range files {
		t.Run(f.Name, func(t *testing.T) {
			for _, c := range f.Commands {
				var wantErr bool
				switch {
				case c.Type == "module",
					c.Type == "assert_invalid",
					c.Type == "assert_unlinkable",
					c.Type == "assert_uninstantiable":
					wantErr = false
				case c.Type == "assert_malformed" && c.ModuleType == "binary":
					wantErr = true
				default:
					total.skipped++
					continue
				}

				key := fmt.Sprintf("%s:%d", f.Name, c.Line)
				data, err := f.ReadModule(c)
				if err != nil {
					t.Errorf("%s: %v", key, err)
					continue
				}

				err = decodeModule(data)
				failed := (err != nil) != wantErr
				if reason, ok := knownFailures[key]; ok {
					delete(stale, key)
					total.known++
					if !failed {
						t.Errorf("%s is listed as a known failure (%s) but now passes; remove it", key, reason)
					}
					continue
				}
				total.ran++
				switch {
				case wantErr && err == nil:
					t.Errorf("%s: %s should be malformed (%q) but decoded", key, c.Type, c.Text)
				case !wantErr && err != nil:
					t.Errorf("%s: %s should decode: %v", key, c.Type, err)
				}
			}
		})
	}
	t.Logf("%d commands checked, %d known failures, %d skipped (text modules and execution commands)", total.ran, total.known, total.skipped)
	for key := range stale {
		t.Errorf("knownFailures lists %s, which is not in the converted tests", key)
	}
}
