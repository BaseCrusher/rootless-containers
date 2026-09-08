package main

import "testing"

func TestExpandAppendsAfterExisting(t *testing.T) {
	env := []string{
		"COREDNSARECORDS_MAIN=1.2.3.4, 5.6.7.8",
		"COREDNS_MAIN__records___AT__1=60 IN SOA ns hostmaster 1 60 60 60 60",
		"COREDNS_MAIN__records___AT__2=60 IN NS ns.example.org.",
		"COREDNS_MAIN_ZONE=example.org",
	}
	want := map[string]string{
		"COREDNS_MAIN__records___AT__3": "IN A 1.2.3.4",
		"COREDNS_MAIN__records___AT__4": "IN A 5.6.7.8",
	}
	got := expand(env)
	if len(got) != len(want) {
		t.Fatalf("got %d entries, want %d: %v", len(got), len(want), got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
}

func TestExpandStartsAtOneWhenNoRecords(t *testing.T) {
	got := expand([]string{"COREDNSARECORDS_WEB=10.0.0.1"})
	if got["COREDNS_WEB__records___AT__1"] != "IN A 10.0.0.1" {
		t.Errorf("got %v", got)
	}
}
