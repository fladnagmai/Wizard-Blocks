package stratum

import "fmt"

// ConnLimits caps miner connections: in total and from one IP address.
// They can be changed at runtime (Settings); a change applies to new
// connections only: lowering a limit never drops miners already connected.
type ConnLimits struct {
	MaxConnections int `json:"max_connections"`
	MaxPerIP       int `json:"max_connections_per_ip"`
}

// MaxConnLimit is the highest value accepted for either limit (each
// connection needs a file descriptor and ~60 kB in the engine).
const MaxConnLimit = 100000

// Validate checks the limits.
func (l ConnLimits) Validate() error {
	if l.MaxConnections < 1 || l.MaxConnections > MaxConnLimit {
		return fmt.Errorf("max connections must be between 1 and %d", MaxConnLimit)
	}
	if l.MaxPerIP < 1 || l.MaxPerIP > MaxConnLimit {
		return fmt.Errorf("max connections per IP must be between 1 and %d", MaxConnLimit)
	}
	if l.MaxPerIP > l.MaxConnections {
		return fmt.Errorf("max connections per IP (%d) cannot exceed max connections (%d)", l.MaxPerIP, l.MaxConnections)
	}
	return nil
}

// ConnLimits returns the live connection limits.
func (s *Server) ConnLimits() ConnLimits {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.limits
}

// SetConnLimits validates and applies new connection limits (to new
// connections; existing ones are kept).
func (s *Server) SetConnLimits(l ConnLimits) error {
	if err := l.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	s.limits = l
	open := len(s.sessions)
	s.mu.Unlock()
	s.log.Info("connection limits applied", "max_connections", l.MaxConnections, "max_per_ip", l.MaxPerIP, "open_now", open)
	return nil
}
