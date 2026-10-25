package decode

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"slices"

	"github.com/DanielleB-R/wombat/wasm"
)

func ReadSections(reader *bytes.Reader) (*wasm.RawModule, error) {
	var module wasm.RawModule

	magicNumberBuffer := make([]byte, 4)
	n, err := reader.Read(magicNumberBuffer)
	if n != 4 || err != nil {
		return nil, errors.New("file too short")
	}
	if !slices.Equal(magicNumberBuffer, []byte{0x00, 0x61, 0x73, 0x6d}) {
		return nil, errors.New("magic number mismatch")
	}

	versionBuffer := make([]byte, 4)
	n, err = reader.Read(versionBuffer)
	if n != 4 || err != nil {
		return nil, errors.New("file too short")
	}
	if !slices.Equal(versionBuffer, []byte{0x01, 0x00, 0x00, 0x00}) {
		return nil, errors.New("version mismatch")
	}

	lastId := byte(0)

	for reader.Len() > 0 {
		id, err := reader.ReadByte()
		if err != nil {
			return nil, fmt.Errorf("error reading section id: %w", err)
		}

		if id != 0 && id <= lastId {
			return nil, errors.New("section out of order")
		}
		if id > 11 {
			return nil, errors.New("unknown section type")
		}
		lastId = id

		length, err := ReadLEB128U32(reader)
		if err != nil {
			return nil, fmt.Errorf("error reading section length: %w", err)
		}

		if length > uint32(reader.Len()) {
			return nil, errors.New("section length too long")
		}

		contents := make([]byte, length)
		n, err := reader.Read(contents)
		if n != int(length) {
			return nil, errors.New("contents unavailable")
		}
		if err != nil && err != io.EOF {
			return nil, fmt.Errorf("error reading section contents, %w", err)
		}

		if id == 0 {
			continue
		}

		module.Sections = append(module.Sections, wasm.RawSection{Id: id, Length: length, Contents: contents})
	}

	return &module, nil
}

func DecodeSections(raw *wasm.RawModule) (*wasm.Module, error) {
	var module wasm.Module

	for _, rawSection := range raw.Sections {
		switch rawSection.Id {
		case 1:
			typeSection, err := decodeTypeSection(rawSection.Contents)
			if err != nil {
				return nil, fmt.Errorf("error decoding type section, %w", err)
			}
			module.Type = typeSection
			// default:
			// 	return nil, errors.New("unknown section type")
		}
	}

	return &module, nil
}

func decodeTypeSection(contents []byte) (*wasm.TypeSection, error) {
	reader := bytes.NewReader(contents)

	length, err := readLength(reader)
	if err != nil {
		return nil, err
	}

	functionTypes := make([]wasm.FunctionType, length)

	for i := range length {
		prefix, err := reader.ReadByte()
		if err != nil {
			return nil, err
		}
		if prefix != 0x60 {
			return nil, errors.New("invalid function type")
		}

		paramsLength, err := readLength(reader)
		if err != nil {
			return nil, err
		}

		parameters := make([]wasm.ValType, paramsLength)

		for j := range paramsLength {
			input, err := reader.ReadByte()
			if err != nil {
				return nil, err
			}
			valType := wasm.ValType(input)
			if !valType.IsValid() {
				return nil, errors.New("invalid value type")
			}
			parameters[j] = valType
		}

		resultsLength, err := readLength(reader)
		if err != nil {
			return nil, err
		}

		results := make([]wasm.ValType, resultsLength)

		for j := range resultsLength {
			input, err := reader.ReadByte()
			if err != nil {
				return nil, err
			}
			valType := wasm.ValType(input)
			if !valType.IsValid() {
				return nil, errors.New("invalid value type")
			}
			results[j] = valType
		}

		functionTypes[i] = wasm.FunctionType{
			Parameters: parameters,
			Results:    results,
		}
	}

	if reader.Len() != 0 {
		return nil, errors.New("contents didn't fill section")
	}

	return &wasm.TypeSection{FunctionTypes: functionTypes}, nil
}

func readLength(reader *bytes.Reader) (int32, error) {
	length, err := ReadLEB128I32(reader)
	if err != nil {
		return 0, fmt.Errorf("error reading length: %w", err)
	}

	if length > int32(reader.Len()) {
		return 0, errors.New("length too long")
	}
	return length, nil
}
