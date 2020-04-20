package simplesocks

import (
	"bufio"
	"fmt"
	"io"

	"github.com/p4gefau1t/trojan-go/common"
	"github.com/p4gefau1t/trojan-go/conf"
	"github.com/p4gefau1t/trojan-go/log"
	"github.com/p4gefau1t/trojan-go/protocol"
	"github.com/p4gefau1t/trojan-go/stat"
)

type SimpleSocksConnSession struct {
	protocol.ConnSession
	protocol.NeedMeter
	protocol.HasRequest

	config        *conf.GlobalConfig
	request       *protocol.Request
	bufReadWriter *bufio.ReadWriter
	conn          io.ReadWriteCloser
	passwordHash  string
	meter         stat.TrafficMeter
	recv          uint64
	sent          uint64
}

func (m *SimpleSocksConnSession) Read(p []byte) (int, error) {
	n, err := m.bufReadWriter.Read(p)
	m.recv += uint64(n)
	return n, err
}

func (m *SimpleSocksConnSession) Write(p []byte) (int, error) {
	n, err := m.bufReadWriter.Write(p)
	m.bufReadWriter.Flush()
	m.sent += uint64(n)
	return n, err
}

func (m *SimpleSocksConnSession) Close() error {
	m.meter.Count(m.passwordHash, m.sent, m.recv)
	log.Info("mux conn to", m.request, "closed", "sent:", common.HumanFriendlyTraffic(m.sent), "recv:", common.HumanFriendlyTraffic(m.recv))
	return m.conn.Close()
}

func (m *SimpleSocksConnSession) SetMeter(meter stat.TrafficMeter) {
	m.meter = meter
}

func (m *SimpleSocksConnSession) GetRequest() *protocol.Request {
	return m.request
}

func (m *SimpleSocksConnSession) parseRequest() error {
	cmd, err := m.bufReadWriter.ReadByte()
	if err != nil {
		return common.NewError("failed to read cmd").Base(err)
	}

	network := "tcp"
	switch protocol.Command(cmd) {
	case protocol.Connect, protocol.Mux:
		network = "tcp"
	case protocol.Associate:
		network = "udp"
	default:
		return common.NewError(fmt.Sprintf("invalid command %d", cmd))
	}

	req, err := protocol.ParseAddress(m.bufReadWriter)
	if err != nil {
		return err
	}
	req.NetworkType = network
	req.Command = protocol.Command(cmd)
	m.request = req
	return nil
}

func (m *SimpleSocksConnSession) writeRequest(req *protocol.Request) error {
	m.bufReadWriter.WriteByte(byte(req.Command))
	common.Must(protocol.WriteAddress(m.bufReadWriter, req))
	m.request = req
	return m.bufReadWriter.Flush()
}

func NewInboundSimpleSocksConnSession(conn io.ReadWriteCloser, passwordHash string) (protocol.ConnSession, error) {
	m := &SimpleSocksConnSession{
		conn:          conn,
		bufReadWriter: common.NewBufReadWriter(conn),
		meter:         &stat.EmptyTrafficMeter{},
	}
	if err := m.parseRequest(); err != nil {
		return nil, common.NewError("failed to parse mux request").Base(err)
	}
	return m, nil
}

func NewOutboundConnSession(req *protocol.Request, conn io.ReadWriteCloser) (protocol.ConnSession, error) {
	m := &SimpleSocksConnSession{
		conn:          conn,
		bufReadWriter: common.NewBufReadWriter(conn),
		meter:         &stat.EmptyTrafficMeter{},
		passwordHash:  "LOCAL_USER",
	}
	if err := m.writeRequest(req); err != nil {
		return nil, common.NewError("failed to write mux request").Base(err)
	}
	return m, nil
}