package controller

import (
	"errors"
	"fmt"
	"net/netip"
)

// ErrPoolExhausted means every subnet in the VM address pool is allocated.
var ErrPoolExhausted = errors.New("VM address pool is exhausted")

// firewallPriorityBase leaves lower priorities for platform rule collections managed by IaC.
const firewallPriorityBase = 1000

// VMPool allocates fixed-size subnets for workspace VMs from one address range.
type VMPool struct {
	Prefix       netip.Prefix
	SubnetLength int
}

// ParseVMPool parses a pool such as "10.240.0.0/16" with per-workspace subnets of length bits.
func ParseVMPool(cidr string, length int) (*VMPool, error) {
	p, err := netip.ParsePrefix(cidr)
	if err != nil {
		return nil, fmt.Errorf("vm address pool: %w", err)
	}
	if !p.Addr().Is4() {
		return nil, errors.New("vm address pool must be IPv4")
	}
	if p.Masked() != p {
		return nil, fmt.Errorf("vm address pool %s is not a network address (did you mean %s?)", p, p.Masked())
	}
	// Azure reserves 5 addresses per subnet, so /29 is the smallest useful size.
	if length < p.Bits() || length > 29 {
		return nil, fmt.Errorf("subnet length /%d must be between /%d and /29", length, p.Bits())
	}
	return &VMPool{Prefix: p, SubnetLength: length}, nil
}

// Size is the number of subnets the pool holds.
func (v *VMPool) Size() int { return 1 << (v.SubnetLength - v.Prefix.Bits()) }

// Subnet returns the i-th subnet of the pool.
func (v *VMPool) Subnet(i int) netip.Prefix {
	base := v.Prefix.Addr().As4()
	n := uint32(base[0])<<24 | uint32(base[1])<<16 | uint32(base[2])<<8 | uint32(base[3])
	n += uint32(i) << (32 - v.SubnetLength)
	return netip.PrefixFrom(netip.AddrFrom4([4]byte{byte(n >> 24), byte(n >> 16), byte(n >> 8), byte(n)}), v.SubnetLength)
}

// Allocate returns the lowest free subnet and a firewall priority unique to it.
// used holds address prefixes already allocated to other workspaces.
func (v *VMPool) Allocate(used map[string]bool) (*VMNetworkAllocation, error) {
	for i := 0; i < v.Size(); i++ {
		s := v.Subnet(i)
		if !used[s.String()] {
			return &VMNetworkAllocation{AddressPrefix: s.String(), FirewallPriority: int32(firewallPriorityBase + i)}, nil
		}
	}
	return nil, ErrPoolExhausted
}

// VMNetworkAllocation is the result of Allocate.
type VMNetworkAllocation struct {
	AddressPrefix    string
	FirewallPriority int32
}
