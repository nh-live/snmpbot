package snmp

import (
    "fmt"
    "log"
    "net"
    "os"
    "time"
    wapsnmp "github.com/cdevr/WapSNMP"
)

type Trap struct {
    Agent       net.IP

    SysUpTime   time.Duration
    SnmpTrapOID OID

    Objects     []VarBind
}

func parseTrapV2(pdu PDU) (trap Trap, err error) {
    if len(pdu.VarBinds) < 2 {
        return trap, fmt.Errorf("short varbinds")
    }

    if varSysUpTime := pdu.VarBinds[0]; SNMPv2_sysUpTime.Index(OID(varSysUpTime.Name)) == nil {
        return trap, fmt.Errorf("incorrect sysUpTime")
    } else if sysUpTime, ok := varSysUpTime.Value.(time.Duration); ! ok {
        return trap, fmt.Errorf("invalid sysUpTime")
    } else {
        trap.SysUpTime = sysUpTime
    }

    if varSnmpTrapOID := pdu.VarBinds[1]; SNMPv2_snmpTrapOID.Index(OID(varSnmpTrapOID.Name)) == nil {
        return trap, fmt.Errorf("incorrect snmpTrapOID")
    } else if snmpTrapOID, ok := varSnmpTrapOID.Value.(wapsnmp.Oid); !ok {
        return trap, fmt.Errorf("invalid snmpTrapOID")
    } else {
        trap.SnmpTrapOID = OID(snmpTrapOID)
    }

    trap.Objects = pdu.VarBinds[2:]

    return trap, nil
}

// Parse SNMPv1 Trap-PDU
// per RFC1908#3.1.2
func parseTrapV1(trapPdu TrapPDU) (trap Trap, err error) {
    trap.SysUpTime = trapPdu.TimeStamp

    switch trapPdu.GenericTrap {
    case TrapColdStart:
        trap.SnmpTrapOID = SNMPv2_coldStart
    case TrapWarmStart:
        trap.SnmpTrapOID = SNMPv2_warmStart
    case TrapLinkDown:
        trap.SnmpTrapOID = If_linkDown
    case TrapLinkUp:
        trap.SnmpTrapOID = If_linkUp
    case TrapAuthenticationFailure:
        trap.SnmpTrapOID = SNMPv2_authenticationFailure
        // TrapEgpNeighborLoss
    case TrapEnterpriseSpecific:
        trap.SnmpTrapOID = OID(trapPdu.Enterprise).define(0, trapPdu.SpecificTrap)
    default:
        trap.SnmpTrapOID = SNMPv2MIB.define(1, 5, int(trapPdu.GenericTrap) + 1) // XXX: oh my :)
    }

    trap.Objects = trapPdu.VarBinds

    return trap, nil
}

// Listen and dispatch traps
type TrapListen struct {
    udpConn    *net.UDPConn
    udpSize     uint

    log         *log.Logger
    listener    chan Trap
}

func NewTrapListen(addr string) (*TrapListen, error) {
    trapListen := &TrapListen{
        udpSize:    UDP_SIZE,
        listener:   make(chan Trap),
    }

    if udpAddr, err := net.ResolveUDPAddr("udp", addr); err != nil {
        return nil, err
    } else if udpConn, err := net.ListenUDP("udp", udpAddr); err != nil {
        return nil, err
    } else {
        trapListen.udpConn = udpConn
    }

    trapListen.log = log.New(os.Stderr, fmt.Sprintf("snmp.TrapListen %s: ", trapListen), 0)

    // start listening
    go trapListen.listen()

    return trapListen, nil
}

func (self TrapListen) String() string {
    return fmt.Sprintf("%v", self.udpConn.LocalAddr())
}

func (self *TrapListen) recv() (addr *net.UDPAddr, packet Packet, packetPdu []interface{}, err error) {
    // recv
    buf := make([]byte, self.udpSize)

    size, addr, err := self.udpConn.ReadFromUDP(buf)
    if err != nil {
        return nil, packet, nil, err
    } else if size == 0 {
        return nil, packet, nil, nil
    }

    // parse
    if packet, packetPdu, err := parsePacket(buf[:size]); err != nil {
        return nil, packet, nil, err
    } else {
        return addr, packet, packetPdu, nil
    }
}

// goroutine to read packets, decode and dispatch them
func (self *TrapListen) listen() {
    for {
        if recvAddr, packet, packetPdu, err := self.recv(); err != nil {
            self.log.Printf("listen recv: %s\n", err)
        } else {
            switch packet.PduType {
            case wapsnmp.AsnTrapV1:
                if pdu, err := parseTrapPDU(packetPdu); err != nil {
                    self.log.Printf("listen parseTrapPDU: invalid TrapV1 pdu: %s\n", err)
                } else if trap, err := parseTrapV1(pdu); err != nil {
                    self.log.Printf("listen parseTrapV2: invalid TrapV2 trap: %s\n", err)
                } else {
                    self.log.Printf("listen trapV1: %s %+v %+v: %+v\n", recvAddr, packet, pdu, trap)

                    self.listenTrap(trap, recvAddr.IP)
                }

            case wapsnmp.AsnTrapV2:
                if pdu, err := parsePDU(packetPdu); err != nil {
                    self.log.Printf("listen parsePDU: invalid TrapV2 pdu: %s\n", err)
                } else if trap, err := parseTrapV2(pdu); err != nil {
                    self.log.Printf("listen parseTrapV2: invalid TrapV2 trap: %s\n", err)
                } else {
                    self.log.Printf("listen trapV2: %s %+v %+v: %+v\n", recvAddr, packet, pdu, trap)

                    self.listenTrap(trap, recvAddr.IP)
                }

            default:
                self.log.Printf("listen: ignore unkown packet type: %v\n", packet.PduType)
            }
        }
    }
}

// Report a trap
func (self *TrapListen) listenTrap(trap Trap, agent net.IP) {
    trap.Agent = agent

    self.listener <- trap
}

// Recv Traps on the returned channel.
// If multiple goroutines subscribe, Traps will be round-robin'd.
func (self *TrapListen) Listen() chan Trap {
    return self.listener
}