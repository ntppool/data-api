package ntpdb

import (
	"strconv"
	"strings"
)

func (m *Monitor) DisplayName() string {
	switch {
	case len(m.Name) > 0:
		return m.Name
	case m.TlsName.Valid && len(m.TlsName.String) > 0:
		name := m.TlsName.String
		if idx := strings.Index(name, "."); idx > 0 {
			name = name[0:idx]
		}
		return name
	case len(m.Location) > 0:
		return m.Location + " (" + strconv.Itoa(int(m.ID)) + ")" // todo: IDToken instead of ID
	default:
		return strconv.Itoa(int(m.ID)) // todo: IDToken
	}
}
