package rulebase

import "testing"

func TestParseContactVCard_ExtractsNFNWAID(t *testing.T) {
	vcard := "BEGIN:VCARD\nVERSION:3.0\nN:Arien;INFA;;;\nFN:INFA Arien\nTEL;type=Mobile;waid=6281275401460:+62 812-7540-1460\nEND:VCARD"

	info, err := parseContactVCard(vcard)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if info == nil {
		t.Fatalf("expected parsed info, got nil")
	}

	if info.N != "Arien;INFA;;;" {
		t.Fatalf("unexpected N: %q", info.N)
	}
	if info.FN != "INFA Arien" {
		t.Fatalf("unexpected FN: %q", info.FN)
	}
	if info.WAID != "6281275401460" {
		t.Fatalf("unexpected WAID: %q", info.WAID)
	}
}

func TestParseContactVCard_FallbackWAIDFromTELValue(t *testing.T) {
	vcard := "BEGIN:VCARD\nVERSION:3.0\nN:Doe;Jane;;;\nFN:Jane Doe\nTEL;type=Cell:+62 811-2222-3333\nEND:VCARD"

	info, err := parseContactVCard(vcard)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if info.WAID != "6281122223333" {
		t.Fatalf("unexpected WAID fallback: %q", info.WAID)
	}
}
