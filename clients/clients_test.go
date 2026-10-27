package clients

import (
	"io/fs"
	"testing"

	"github.com/donseba/go-partial/exp/flash"
	"github.com/donseba/go-partial/exp/interactions"
	"github.com/donseba/go-partial/exp/sse"
)

func TestFSHoldsEveryClient(t *testing.T) {
	files := FS()

	for name, script := range map[string][]byte{
		"partial-flash.js":        flash.ClientScript(),
		"partial-sse.js":          sse.ClientScript(),
		"partial-interactions.js": interactions.ClientScript(),
	} {
		data, err := fs.ReadFile(files, name)
		if err != nil || string(data) != string(script) {
			t.Fatalf("%s: %d bytes, %v", name, len(data), err)
		}
	}

	entries, err := fs.ReadDir(files, ".")
	if err != nil || len(entries) != 3 {
		t.Fatalf("ReadDir() = %v, %v", entries, err)
	}
}

func TestAllNamesEveryClientInFS(t *testing.T) {
	all := All()
	if len(all) != 3 {
		t.Fatalf("All() = %v", all)
	}

	for _, client := range all {
		if client.Import+".js" != client.File {
			t.Errorf("%s: file %s", client.Import, client.File)
		}

		if _, err := fs.Stat(FS(), client.File); err != nil {
			t.Errorf("%s: %v", client.File, err)
		}
	}
}
