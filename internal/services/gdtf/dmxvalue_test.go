package gdtf

import "testing"

func TestParseDMXValue(t *testing.T) {
	tests := []struct {
		in      string
		ok      bool
		wantErr bool
		want    dmxValue
	}{
		{in: "0/1", ok: true, want: dmxValue{value: 0, bytes: 1}},
		{in: "255/1", ok: true, want: dmxValue{value: 255, bytes: 1}},
		{in: "128/1s", ok: true, want: dmxValue{value: 128, bytes: 1, shift: true}},
		{in: "65535/2", ok: true, want: dmxValue{value: 65535, bytes: 2}},
		{in: " 12/3 ", ok: true, want: dmxValue{value: 12, bytes: 3}},
		{in: "300/1", ok: true, want: dmxValue{value: 255, bytes: 1}}, // clamped
		{in: "7", ok: true, want: dmxValue{value: 7, bytes: 1}},       // bare integer
		{in: "", ok: false},
		{in: "None", ok: false},
		{in: "none", ok: false},
		{in: "abc/1", wantErr: true},
		{in: "1/0", wantErr: true},
		{in: "1/5", wantErr: true},
		{in: "1/xs", wantErr: true},
		{in: "-1/1", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, ok, err := parseDMXValue(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if ok && got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDMXValueTo(t *testing.T) {
	tests := []struct {
		in    string
		bytes int
		want  uint64
	}{
		// Byte mirroring (default).
		{"255/1", 1, 255},
		{"255/1", 2, 65535},
		{"128/1", 2, 0x8080},
		{"128/1", 3, 0x808080},
		{"0/1", 2, 0},
		// Byte shifting.
		{"255/1s", 2, 65280},
		{"128/1s", 2, 0x8000},
		{"1/1s", 3, 0x010000},
		// Same resolution.
		{"32768/2", 2, 32768},
		// Narrowing keeps the most significant byte(s).
		{"65535/2", 1, 255},
		{"32768/2", 1, 128},
		// Mirroring a 2-byte value to 3 bytes repeats its bytes.
		{"4660/2", 3, 0x123412},
		// Resolution below 1 is treated as 1.
		{"200/1", 0, 200},
	}
	for _, tt := range tests {
		v, ok, err := parseDMXValue(tt.in)
		if err != nil || !ok {
			t.Fatalf("parseDMXValue(%q): ok=%v err=%v", tt.in, ok, err)
		}
		if got := v.to(tt.bytes); got != tt.want {
			t.Errorf("%s to %d bytes = %#x, want %#x", tt.in, tt.bytes, got, tt.want)
		}
	}
	if _, _, err := parseDMXValue("0x1234"); err == nil {
		t.Error("hex DMX values should be rejected")
	}
}

func TestSplitBytes(t *testing.T) {
	got := splitBytes(0x8040, 2)
	if got[0] != 0x80 || got[1] != 0x40 {
		t.Errorf("splitBytes(0x8040, 2) = %v", got)
	}
	got = splitBytes(0x010203, 3)
	if got[0] != 1 || got[1] != 2 || got[2] != 3 {
		t.Errorf("splitBytes(0x010203, 3) = %v", got)
	}
}
