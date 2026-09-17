package registry

import (
	"os"
	"testing"
)

func TestParseCatalog(t *testing.T) {
	raw := []byte(`{
  "schema": 1,
  "tools": [
    {
      "name": "mise",
      "category": "version-manager",
      "binaries": ["mise"],
      "init": true,
      "install": [{"kind": "script", "script": "curl -fsSL https://mise.run | sh"}]
    },
    {
      "name": "build",
      "category": "chained",
      "binaries": ["build"],
      "init": false,
      "install": [{
        "kind": "releases",
        "slug": "build",
        "asset": "build-{version}-{os}-{arch}.tar.gz",
        "exe_in": "build",
        "arch_map": {"amd64": "x86_64"}
      }]
    },
    {
      "name": "clerk-zig",
      "category": "sdk",
      "init": false,
      "language": "zig",
      "install": [{"kind": "sdks", "slug": "clerk-zig"}]
    }
  ]
}`)
	r, err := ParseCatalog(raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := r.Get("mise"); !ok {
		t.Fatal("missing mise")
	}
	build, ok := r.Get("build")
	if !ok {
		t.Fatal("missing build")
	}
	if build.Init {
		t.Fatal("build should not be in default init")
	}
	if len(build.Install) != 1 || build.Install[0].Kind != MethodReleases {
		t.Fatalf("build install = %+v", build.Install)
	}
	if build.Install[0].ArchMap["amd64"] != "x86_64" {
		t.Fatalf("arch map = %#v", build.Install[0].ArchMap)
	}
	sdk, ok := r.Get("clerk-zig")
	if !ok {
		t.Fatal("missing clerk-zig")
	}
	if sdk.Language != "zig" || sdk.Install[0].Kind != MethodSdks {
		t.Fatalf("sdk = %+v", sdk)
	}
}

func TestParseShippedAPICatalog(t *testing.T) {
	data, err := os.ReadFile("../../../api/priv/climan/tools.json")
	if err != nil {
		t.Skip(err)
	}
	r, err := ParseCatalog(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"mise", "build", "clerk-zig"} {
		if _, ok := r.Get(name); !ok {
			t.Errorf("shipped catalog missing %q", name)
		}
	}
}

func TestParseCatalogRejectsUnknownKind(t *testing.T) {
	_, err := ParseCatalog([]byte(`{"schema":1,"tools":[{"name":"x","install":[{"kind":"nope"}]}]}`))
	if err == nil {
		t.Fatal("expected error")
	}
}
