package preset

import (
	"encoding/base64"
	"os"
	"strings"
	"testing"
)

func TestDecodeJSONFixture(t *testing.T) {
	b, err := os.ReadFile("testdata/valid.json")
	if err != nil {
		t.Fatal(err)
	}
	p, err := DecodeJSON(b)
	if err != nil {
		t.Fatalf("decode preset json: %v", err)
	}
	if p.Config.WANInterface != "eth0" || len(p.Config.OpenPorts) != 1 || !p.Config.Docker.Enabled {
		t.Fatalf("unexpected preset config: %#v", p.Config)
	}
}

func TestDecodeStringRawBase64URL(t *testing.T) {
	b, err := os.ReadFile("testdata/valid.json")
	if err != nil {
		t.Fatal(err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(b)
	p, err := DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode preset string: %v", err)
	}
	if p.Config.SSH.DDNSWhitelist.Hosts[0] != "home.example.com" {
		t.Fatalf("unexpected host: %#v", p.Config.SSH.DDNSWhitelist.Hosts)
	}
}

func TestDecodeRejectsUnknownVersion(t *testing.T) {
	_, err := DecodeJSON([]byte(`{"version":2}`))
	if err == nil || !strings.Contains(err.Error(), "unsupported config version 2") {
		t.Fatalf("expected unknown version error, got %v", err)
	}
}

func TestDecodeRejectsUnknownField(t *testing.T) {
	_, err := DecodeJSON([]byte(`{"version":1,"surprise":true}`))
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("expected unknown field error, got %v", err)
	}
}

func TestDecodeRejectsInvalidConfig(t *testing.T) {
	_, err := DecodeJSON([]byte(`{"version":1,"open_ports":[{"protocol":"sctp","port":443}]}`))
	if err == nil || !strings.Contains(err.Error(), "open_ports[0].protocol") {
		t.Fatalf("expected validation error, got %v", err)
	}
}

func TestEncodeRoundTrip(t *testing.T) {
	b, err := os.ReadFile("testdata/valid.json")
	if err != nil {
		t.Fatal(err)
	}
	p, err := DecodeJSON(b)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := Encode(p)
	if err != nil {
		t.Fatalf("encode preset: %v", err)
	}
	decoded, err := DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode encoded preset: %v", err)
	}
	if decoded.Config.WANInterface != p.Config.WANInterface || decoded.Config.OpenPorts[0].Port != 443 {
		t.Fatalf("round trip mismatch: %#v", decoded.Config)
	}
}

func TestExplainIncludesRiskWarnings(t *testing.T) {
	b, err := os.ReadFile("testdata/valid.json")
	if err != nil {
		t.Fatal(err)
	}
	p, err := DecodeJSON(b)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(p.Explain(), "\n")
	for _, want := range []string{"config version: 1", "WAN interface: eth0", "open ports: 1", "risk warnings:", "public tcp port 443", "Docker integration"} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %q in explanation:\n%s", want, text)
		}
	}
}

func FuzzPresetJSONAndBase64(f *testing.F) {
	valid, err := os.ReadFile("testdata/valid.json")
	if err != nil {
		f.Fatal(err)
	}
	f.Add(valid)
	f.Add([]byte(`{"version":1}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		if p, err := DecodeJSON(data); err == nil {
			encoded, err := Encode(p)
			if err != nil {
				t.Fatalf("encode decoded preset: %v", err)
			}
			if _, err := DecodeString(encoded); err != nil {
				t.Fatalf("decode encoded preset: %v", err)
			}
		}
		encoded := base64.RawURLEncoding.EncodeToString(data)
		_, _ = DecodeString(encoded)
	})
}
