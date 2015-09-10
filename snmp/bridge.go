package snmp

// SNMP BRIDGE-MIB implementation

var (
    BridgeMIB       = MIB{OID{1,3,6,1,2,1,17}}
)

type Bridge_FdbIndex struct {
    Address     MacAddress
}

func (self *Bridge_FdbIndex) setIndex (oid OID) error {
    return self.Address.setIndex(oid)
}

func (self Bridge_FdbIndex) String() string {
    return self.Address.String()
}

type Bridge_FdbEntry struct {
    Address     MacAddress  `snmp:"1.3.6.1.2.1.17.4.3.1.1"`
    Port        Integer     `snmp:"1.3.6.1.2.1.17.4.3.1.2"`
    Status      Integer     `snmp:"1.3.6.1.2.1.17.4.3.1.3"`
}

type Bridge_FdbTable map[Bridge_FdbIndex]*Bridge_FdbEntry