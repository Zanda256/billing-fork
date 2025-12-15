package ccbill

import (
	"fmt"
	"net"

	"github.com/doujins-org/doujins-billing/config"
)

var ccbillIPRanges = []*net.IPNet{
	parseCIDR("64.38.212.0/24"),
	parseCIDR("64.38.215.0/24"),
	parseCIDR("64.38.240.0/24"),
	parseCIDR("64.38.241.0/24"),
}

type RESTClient struct {
	config *config.CCBillConfig
}

func (c *RESTClient) Config() *config.CCBillConfig {
	return c.config
}

func NewRESTClient(cfg *config.CCBillConfig) *RESTClient {
	return &RESTClient{
		config: cfg,
	}
}

func parseCIDR(cidr string) *net.IPNet {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		panic(fmt.Sprintf("failed to parse CCBill IP range %s: %v", cidr, err))
	}
	return ipNet
}

func (c *RESTClient) ValidateWebhookIP(clientIP string) error {
	ip := net.ParseIP(clientIP)
	if ip == nil {
		return fmt.Errorf("invalid IP address: %s", clientIP)
	}

	for _, ipRange := range ccbillIPRanges {
		if ipRange.Contains(ip) {
			return nil
		}
	}

	return fmt.Errorf("webhook request from unauthorized IP: %s", clientIP)
}

func (c *RESTClient) ValidateWebhookAuth(clientAccnum, clientSubacc string) error {
	cfg := c.Config()

	if clientAccnum != cfg.ClientAccNum {
		return fmt.Errorf("webhook clientAccnum mismatch: got %s, expected %s", clientAccnum, cfg.ClientAccNum)
	}

	if clientSubacc != cfg.ClientSubAcc {
		return fmt.Errorf("webhook clientSubacc mismatch: got %s, expected %s", clientSubacc, cfg.ClientSubAcc)
	}

	return nil
}
