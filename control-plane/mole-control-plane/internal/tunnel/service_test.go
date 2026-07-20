package tunnel

import "testing"

func TestValidateInput(t *testing.T) {
	protocol, ip, port, err := validateInput(CreateInput{Protocol: " TCP ", InternalAddress: "127.0.0.1:25565"})
	if err != nil {
		t.Fatalf("validate TCP input: %v", err)
	}
	if protocol != "tcp" || ip.String() != "127.0.0.1" || port != 25565 {
		t.Fatalf("unexpected parsed input: %q, %s, %d", protocol, ip, port)
	}
}

func TestValidateInputRejectsInvalidEndpoint(t *testing.T) {
	tests := []CreateInput{
		{Protocol: "icmp", InternalAddress: "127.0.0.1:25565"},
		{Protocol: "tcp", InternalAddress: "localhost:25565"},
		{Protocol: "udp", InternalAddress: "127.0.0.1:0"},
		{Protocol: "udp", InternalAddress: "127.0.0.1"},
	}
	for _, input := range tests {
		if _, _, _, err := validateInput(input); err != ErrInvalidInput {
			t.Fatalf("expected invalid input for %+v, got %v", input, err)
		}
	}
}
