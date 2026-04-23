package sysinfo

import (
	"encoding/binary"
	"unsafe"

	"github.com/EXBO-Community/stalcraft-jvm-optimization/internal/winapi"
)

var procGetSystemFirmwareTable = winapi.Kernel32.NewProc("GetSystemFirmwareTable")

// SMBIOS provider signature 'RSMB' encoded as a DWORD. Win32 builds
// this from the C multi-char constant 'RSMB' which evaluates to
// (R << 24) | (S << 16) | (M << 8) | B on little-endian platforms.
const smbiosProviderRSMB = 0x52534D42

// DMI structure types relevant here.
const (
	dmiMemoryDevice = 17
	dmiEndOfTable   = 127
)

// detectMemSpeedMTs queries the raw SMBIOS table for populated memory
// devices (DMI Type 17) and returns the highest configured memory
// clock speed reported, in MT/s. Returns 0 when the probe fails or no
// populated DIMM is present — the caller treats 0 as "unknown" and
// falls back to the mid memory tier.
//
// ConfiguredMemoryClockSpeed (the actual running speed, driven by
// XMP / DOCP or the SPD default) is preferred over SPD Speed (the
// DIMM's rated capability). On a board without XMP enabled these two
// diverge — DDR4-3200 DIMMs routinely run at 2666 MT/s — and JVM
// tuning must reflect real bandwidth, not marketing numbers.
func detectMemSpeedMTs() int {
	r, _, _ := procGetSystemFirmwareTable.Call(uintptr(smbiosProviderRSMB), 0, 0, 0)
	size := uint32(r)
	if size == 0 {
		return 0
	}
	buf := make([]byte, size)
	r, _, _ = procGetSystemFirmwareTable.Call(
		uintptr(smbiosProviderRSMB),
		0,
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(size),
	)
	if uint32(r) == 0 {
		return 0
	}

	// RawSMBIOSData: Used20CallingMethod, MajorVersion, MinorVersion,
	// DmiRevision (all BYTE), then Length (DWORD), then the packed
	// table data. Header size = 8 bytes.
	if len(buf) < 8 {
		return 0
	}
	tableLen := binary.LittleEndian.Uint32(buf[4:8])
	if tableLen == 0 || 8+int(tableLen) > len(buf) {
		return 0
	}
	table := buf[8 : 8+int(tableLen)]

	best := 0
	for off := 0; off+4 <= len(table); {
		typ := table[off]
		length := int(table[off+1])
		if length < 4 || off+length > len(table) {
			break
		}
		if typ == dmiMemoryDevice {
			if s := memDeviceSpeedMTs(table[off : off+length]); s > best {
				best = s
			}
		}
		// Each DMI record is followed by a string section terminated
		// by a double-null. Advance past the fixed section, then past
		// the string section.
		off += length
		for off+1 < len(table) && !(table[off] == 0 && table[off+1] == 0) {
			off++
		}
		off += 2
		if typ == dmiEndOfTable {
			break
		}
	}
	return best
}

// memDeviceSpeedMTs extracts the best available speed from a single
// DMI Type 17 record. The fixed-section layout used here:
//
//	offset 0x0C (WORD)  Size            — 0 means "no module"
//	offset 0x15 (WORD)  Speed           — rated DIMM speed, MT/s
//	offset 0x20 (WORD)  ConfiguredSpeed — actual running speed, MT/s
//	offset 0x54 (DWORD) ExtendedSpeed           (SMBIOS 3.x)
//	offset 0x58 (DWORD) ExtendedConfiguredSpeed (SMBIOS 3.x)
//
// A WORD value of 0xFFFF in Speed / ConfiguredSpeed is the SMBIOS
// sentinel meaning "read the extended DWORD field instead", used for
// DDR5 speeds above 65 535 MT/s. Zero and absent-field cases return 0
// so the caller knows to skip this DIMM.
func memDeviceSpeedMTs(rec []byte) int {
	if len(rec) < 0x0E {
		return 0
	}
	if size := binary.LittleEndian.Uint16(rec[0x0C:0x0E]); size == 0 {
		return 0 // empty slot
	}

	if len(rec) >= 0x22 {
		if v := binary.LittleEndian.Uint16(rec[0x20:0x22]); v != 0 && v != 0xFFFF {
			return int(v)
		}
		if len(rec) >= 0x5C && binary.LittleEndian.Uint16(rec[0x20:0x22]) == 0xFFFF {
			if v := binary.LittleEndian.Uint32(rec[0x58:0x5C]); v != 0 {
				return int(v)
			}
		}
	}
	if len(rec) >= 0x17 {
		if v := binary.LittleEndian.Uint16(rec[0x15:0x17]); v != 0 && v != 0xFFFF {
			return int(v)
		}
		if len(rec) >= 0x58 && binary.LittleEndian.Uint16(rec[0x15:0x17]) == 0xFFFF {
			if v := binary.LittleEndian.Uint32(rec[0x54:0x58]); v != 0 {
				return int(v)
			}
		}
	}
	return 0
}
