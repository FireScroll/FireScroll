package gossip

type VoidWriter struct{}

func (VoidWriter) Write(b []byte) (int, error) {
	return len(b), nil
}
