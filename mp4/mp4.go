package mp4

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"strings"
)

// See https://www.w3.org/2013/12/byte-stream-format-registry/isobmff-byte-stream-format.html

type AtomType string

const (
	STYP AtomType = "styp"
	MOOF AtomType = "moof"
	FREE AtomType = "free"
	SIDX AtomType = "sidx"
	//
	FTYP AtomType = "ftyp"
	PDIN AtomType = "pdin"
	MOOV AtomType = "moov"
	MFRA AtomType = "mfra"
	MDAT AtomType = "mdat"
	STTS AtomType = "stts"
	STSC AtomType = "stsc"
	STSZ AtomType = "stsz"
	META AtomType = "meta"
	//
	MVHD AtomType = "mvhd"
	TRAK AtomType = "trak"
	UDTA AtomType = "udta"
	IODS AtomType = "iods"
	MVEX AtomType = "mvex"
)

// AtomTypeMap maps the atom type to the atom name
var AtomTypeMap = map[AtomType]string{
	STYP: "Segment Type Box",
	FTYP: "File Type Box",
	PDIN: "Progressive Download Information Box",
	MOOV: "Movie Box",
	MOOF: "Movie Fragment Box",
	MFRA: "Movie Fragment Random Access Box",
	MDAT: "Media Data Box",
	STTS: "Decoding Time to Sample Box",
	STSC: "Sample to Chunk Box",
	STSZ: "Sample Size Box",
	META: "Metadata Box",
	//
	MVHD: "Movie Header Box",
	TRAK: "Track Box",
	UDTA: "User Data Box",
	IODS: "Initial Object Descriptor Box",
	MVEX: "Movie Extends Box",
	FREE: "Reserved Space Box",
	SIDX: "Segment Index Box",
}

// Atom represents an atom in an ISO BMFF container.
type Atom struct {
	Size     uint32
	TypeCode [4]byte
	Data     []byte
}

func (a *Atom) Type() AtomType {
	return AtomType(a.TypeCode[:])
}

func (a *Atom) Write(w io.Writer) (int64, error) {
	var n int64
	// if _, err := w.Write(a.Type[:]); err != nil {
	// 	return n, err
	// }
	// n += 4
	if _, err := w.Write(a.Data); err != nil {
		return n, err
	}
	n += int64(len(a.Data))
	return n, nil
}

// STYPData represents the styp atom in an ISO BMFF container.
type STYPData struct {
	MajorBrand       [4]byte
	MinorVersion     uint32
	CompatibleBrands [][4]byte
}

func (data *STYPData) String() string {
	var b strings.Builder

	fmt.Fprintf(&b, "Major brand: %s\n", string(data.MajorBrand[:]))
	fmt.Fprintf(&b, "Minor version: %d\n", data.MinorVersion)
	fmt.Fprintf(&b, "Compatible brands:")
	for _, brand := range data.CompatibleBrands {
		fmt.Fprintf(&b, " %s", string(brand[:]))
	}
	fmt.Fprintln(&b)

	return b.String()
}

// ParseAtoms parses the atoms in an ISO BMFF container.
func ParseAtoms(r io.Reader) ([]Atom, error) {
	var atoms []Atom
	for {
		// Read the size and type of the atom.
		var atom Atom
		if err := binary.Read(r, binary.BigEndian, &atom.Size); err != nil {
			if err == io.EOF {
				break
			}
			return atoms, fmt.Errorf("failed to read atom size: %w", err)
		}
		if _, err := io.ReadFull(r, atom.TypeCode[:]); err != nil {
			return atoms, fmt.Errorf("failed to read atom type: %w", err)
		}

		// Read the data of the atom.
		if atom.Size == 1 {
			// The size is 64 bits.
			var size uint64
			if err := binary.Read(r, binary.BigEndian, &size); err != nil {
				return atoms, fmt.Errorf("failed to read extended atom size: %w", err)
			}
			atom.Data = make([]byte, size-16)
		} else {
			// The size is 32 bits.
			atom.Data = make([]byte, atom.Size-8)
		}
		if _, err := io.ReadFull(r, atom.Data); err != nil {
			return atoms, fmt.Errorf("failed to read atom data: %w", err)
		}

		atoms = append(atoms, atom)
	}
	return atoms, nil
}

func ParseMP4(inPath, outPath string) error {
	// Open the input file.
	f, err := os.Open(inPath)
	if err != nil {
		return fmt.Errorf("failed to open input file: %s - %v\n", inPath, err)
	}
	defer f.Close()

	// Parse the atoms in the input file.
	atoms, err := ParseAtoms(f)
	if err != nil {
		return fmt.Errorf("failed to parse atoms: %v\n", err)
	}

	// Iterate over the atoms and extract the raw data.
	for _, atom := range atoms {
		// Skip atoms that are not 'mdat'.
		if string(atom.TypeCode[:]) != "mdat" {
			fmt.Println("skipping atom", string(atom.TypeCode[:]))
			continue
		}

		// Write the raw data to the output file.
		f, err := os.Create(outPath)
		if err != nil {
			return fmt.Errorf("failed to create output file: %s - %v\n", outPath, err)
		}
		defer f.Close()
		if _, err := f.Write(atom.Data); err != nil {
			return fmt.Errorf("failed to write output file: %v\n", err)
		}
	}

	return nil
}

// ParseSTYP parses the styp atom in an ISO BMFF container.
func (a *Atom) ParseSTYP() (*STYPData, error) {
	if a.Type() != STYP {
		return nil, fmt.Errorf("atom is not styp")
	}
	r := bytes.NewReader(a.Data)

	// Read the major brand and minor version.
	var styp STYPData
	if err := binary.Read(r, binary.BigEndian, &styp.MajorBrand); err != nil {
		return nil, fmt.Errorf("failed to read major brand: %w", err)
	}
	if err := binary.Read(r, binary.BigEndian, &styp.MinorVersion); err != nil {
		return nil, fmt.Errorf("failed to read minor version: %w", err)
	}

	// Read the compatible brands.
	for {
		var brand [4]byte
		if _, err := io.ReadFull(r, brand[:]); err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("failed to read compatible brand: %w", err)
		}
		styp.CompatibleBrands = append(styp.CompatibleBrands, brand)
	}

	return &styp, nil
}

// SIDXData represents the data in an sidx atom.
type SIDXData struct {
	Version              byte
	Flags                uint32
	ReferenceID          uint32
	TimeScale            uint32
	EarliestPresentation uint64
	FirstOffset          uint64
	Reserved             uint16
	ReferenceCount       uint16
	References           []SIDXReference
}

// SIDXReference represents a reference to media data in an sidx atom.
type SIDXReference struct {
	ReferenceType            uint32
	ReferenceSize            uint32
	SubsegmentDuration       uint32
	StartsWithSAP            uint32
	SAPType                  uint32
	SAPDeltaTime             uint32
	StartsWithSAPFragment    bool
	SAPFragmentNumber        uint32
	SAPFragmentDuration      uint32
	SAPFragmentNumberIsValid bool
}

// ParseSIDX parses the data in an sidx atom.
func (a *Atom) ParseSIDX() (*SIDXData, error) {
	r := bytes.NewReader(a.Data)

	// Read the version, flags, and reference ID.
	var data SIDXData
	if err := binary.Read(r, binary.BigEndian, &data.Version); err != nil {
		return nil, fmt.Errorf("failed to read version: %w", err)
	}
	if err := binary.Read(r, binary.BigEndian, &data.Flags); err != nil {
		return nil, fmt.Errorf("failed to read flags: %w", err)
	}
	if err := binary.Read(r, binary.BigEndian, &data.ReferenceID); err != nil {
		return nil, fmt.Errorf("failed to read reference ID: %w", err)
	}

	// Read the time scale, earliest presentation time, and first offset.
	if err := binary.Read(r, binary.BigEndian, &data.TimeScale); err != nil {
		return nil, fmt.Errorf("failed to read time scale: %w", err)
	}
	if err := binary.Read(r, binary.BigEndian, &data.EarliestPresentation); err != nil {
		return nil, fmt.Errorf("failed to read earliest presentation time: %w", err)
	}
	if err := binary.Read(r, binary.BigEndian, &data.FirstOffset); err != nil {
		return nil, fmt.Errorf("failed to read first offset: %w", err)
	}

	// Read the reserved field and reference count.
	if err := binary.Read(r, binary.BigEndian, &data.Reserved); err != nil {
		return nil, fmt.Errorf("failed to read reserved field: %w", err)
	}
	if err := binary.Read(r, binary.BigEndian, &data.ReferenceCount); err != nil {
		return nil, fmt.Errorf("failed to read reference count: %w", err)
	}

	// Read the references.
	for i := uint16(0); i < data.ReferenceCount; i++ {
		var ref SIDXReference

		// Read the reference type and size.
		if err := binary.Read(r, binary.BigEndian, &ref.ReferenceType); err != nil {
			return nil, fmt.Errorf("failed to read reference type: %w", err)
		}
		if err := binary.Read(r, binary.BigEndian, &ref.ReferenceSize); err != nil {
			return nil, fmt.Errorf("failed to read reference size: %w", err)
		}

		// Read the subsegment duration.
		if err := binary.Read(r, binary.BigEndian, &ref.SubsegmentDuration); err != nil {
			return nil, fmt.Errorf("failed to read subsegment duration: %w", err)
		}

		// Read the SAP field.
		if err := binary.Read(r, binary.BigEndian, &ref.StartsWithSAP); err != nil {
			return nil, fmt.Errorf("failed to read SAP field: %w", err)
		}

		// Check if the reference starts with a SAP.
		if ref.StartsWithSAP&0x80000000 != 0 {
			ref.StartsWithSAPFragment = true

			// Read the SAP type and delta time.
			if err := binary.Read(r, binary.BigEndian, &ref.SAPType); err != nil {
				return nil, fmt.Errorf("failed to read SAP")
			}

			if err := binary.Read(r, binary.BigEndian, &ref.SAPDeltaTime); err != nil {
				return nil, fmt.Errorf("failed to read SAP delta time: %w", err)
			}

			// Check if the SAP type is valid.
			if ref.SAPType != 0 && ref.SAPType != 1 {
				return nil, fmt.Errorf("invalid SAP type: %d", ref.SAPType)
			}

			// Read the SAP fragment number and duration.
			if err := binary.Read(r, binary.BigEndian, &ref.SAPFragmentNumber); err != nil {
				return nil, fmt.Errorf("failed to read SAP fragment number: %w", err)
			}
			if err := binary.Read(r, binary.BigEndian, &ref.SAPFragmentDuration); err != nil {
				return nil, fmt.Errorf("failed to read SAP fragment duration: %w", err)
			}
			ref.SAPFragmentNumberIsValid = true
		} else {
			ref.StartsWithSAPFragment = false
		}

		data.References = append(data.References, ref)
	}

	return &data, nil
}
