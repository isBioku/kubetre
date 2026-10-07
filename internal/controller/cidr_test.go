package controller

import "testing"

func TestVMPoolAllocatesLowestFreeSubnet(t *testing.T) {
	p, err := ParseVMPool("10.240.0.0/24", 26)
	if err != nil {
		t.Fatal(err)
	}
	if p.Size() != 4 {
		t.Fatalf("size = %d", p.Size())
	}
	used := map[string]bool{"10.240.0.0/26": true, "10.240.0.128/26": true}
	a, err := p.Allocate(used)
	if err != nil || a.AddressPrefix != "10.240.0.64/26" || a.FirewallPriority != 1001 {
		t.Fatalf("allocation = %+v, %v", a, err)
	}
	used[a.AddressPrefix] = true
	used["10.240.0.192/26"] = true
	if _, err := p.Allocate(used); err != ErrPoolExhausted {
		t.Fatalf("expected exhaustion, got %v", err)
	}
}

func TestVMPoolRejectsBadInput(t *testing.T) {
	for _, c := range []struct {
		cidr string
		len  int
	}{{"10.240.0.1/16", 26}, {"10.240.0.0/16", 8}, {"10.240.0.0/16", 30}, {"fd00::/64", 80}, {"nonsense", 26}} {
		if _, err := ParseVMPool(c.cidr, c.len); err == nil {
			t.Errorf("ParseVMPool(%q, %d) accepted", c.cidr, c.len)
		}
	}
}
