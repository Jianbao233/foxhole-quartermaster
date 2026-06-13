package parser

import (
	"os"
	"strings"
	"testing"
)

func TestParseSample(t *testing.T) {
	raw, err := os.ReadFile("testdata/sample_haibour_v1.txt")
	if err != nil {
		t.Fatalf("read sample: %v", err)
	}
	p, err := Parse(string(raw))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if p.Header.Region != "Umbral Wildwood" {
		t.Errorf("region=%q", p.Header.Region)
	}
	if p.Header.SubRegion != "The Gap" {
		t.Errorf("sub_region=%q", p.Header.SubRegion)
	}
	if p.Header.Building != "海港" {
		t.Errorf("building=%q", p.Header.Building)
	}
	if p.Header.Visibility != "Public" {
		t.Errorf("visibility=%q", p.Header.Visibility)
	}
	if len(p.Items) < 200 {
		t.Errorf("items too few: %d", len(p.Items))
	}
	// 抽几个特征数据点验证
	want := map[string]struct {
		isCrate bool
		qty     int64
	}{
		"基础材料":              {true, 260},
		"建筑材料":              {true, 111},
		"砂砾":                {true, 88},
		"BMS - Ironship":   {false, 25},
		"Strider":          {false, 1},
		"K-81e “Sombre”":   {false, 1},
		"货运集装箱":             {true, 7},
	}
	hit := map[string]bool{}
	for _, it := range p.Items {
		if w, ok := want[it.Name]; ok {
			if it.IsCrate == w.isCrate && it.Quantity == w.qty {
				hit[it.Name] = true
			}
		}
	}
	for k := range want {
		if !hit[k] {
			t.Errorf("missing or wrong row: %q", k)
		}
	}
}

func TestParseHeaderVariants(t *testing.T) {
	cases := []struct {
		name string
		in   string
		ok   bool
	}{
		{"private", "Region - Sub - Building - Private - X: 0.1 Y: 0.2,2026.06.13-06.48.16\nA,1\n", true},
		{"missing_visibility_part", "A - B - C,2026.06.13-06.48.16\nA,1\n", false},
		{"bad_visibility", "A - B - C - Foo - X: 0.1 Y: 0.2,2026.06.13-06.48.16\nA,1\n", false},
		{"bad_timestamp", "A - B - C - Public - X: 0.1 Y: 0.2,abc\nA,1\n", false},
		{"empty", "", false},
		{"only_header", "A - B - C - Public - X: 0.1 Y: 0.2,2026.06.13-06.48.16\n", false},
		{"building_with_dash", "A - B - C - D - Public - X: 0.1 Y: 0.2,2026.06.13-06.48.16\nA,1\n", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Parse(c.in)
			if (err == nil) != c.ok {
				t.Fatalf("ok=%v err=%v", c.ok, err)
			}
		})
	}
}

func TestParseItemErrors(t *testing.T) {
	header := "A - B - C - Public - X: 0.1 Y: 0.2,2026.06.13-06.48.16\n"
	cases := []struct {
		name string
		body string
		ok   bool
	}{
		{"negative", "A,-1\n", false},
		{"non_int", "A,abc\n", false},
		{"crate", "A (箱装),5\n", true},
		{"empty_name", " (箱装),5\n", false},
		{"normal", "A,5\n", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Parse(header + c.body)
			if (err == nil) != c.ok {
				t.Fatalf("ok=%v err=%v", c.ok, err)
			}
		})
	}
}

func TestParseDuplicateRowsTakeLast(t *testing.T) {
	in := "A - B - C - Public - X: 0.1 Y: 0.2,2026.06.13-06.48.16\nA,1\nA,2\n"
	p, err := Parse(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Items) != 1 || p.Items[0].Quantity != 2 {
		t.Fatalf("dup not collapsed: %+v", p.Items)
	}
}

func TestParseCrateAndLooseCoexist(t *testing.T) {
	in := "A - B - C - Public - X: 0 Y: 0,2026.06.13-06.48.16\nLamentum mm.IV,3\nLamentum mm.IV (箱装),4\n"
	p, err := Parse(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Items) != 2 {
		t.Fatalf("expected 2 items, got %+v", p.Items)
	}
	if !strings.HasPrefix(p.Items[0].Name, "Lamentum") || p.Items[0].IsCrate {
		t.Fatalf("loose first wrong: %+v", p.Items[0])
	}
	if !p.Items[1].IsCrate || p.Items[1].Quantity != 4 {
		t.Fatalf("crate row wrong: %+v", p.Items[1])
	}
}