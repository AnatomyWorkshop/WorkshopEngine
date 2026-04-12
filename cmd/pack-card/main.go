// pack-card 将 game.json 打包进 PNG 的 tEXt chunk（keyword = "gw_game"）
// 用法：pack-card -game game.json -cover cover.png -out output.png
package main

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"flag"
	"fmt"
	"hash/crc32"
	"os"
)

func main() {
	gameFile := flag.String("game", "", "game.json 路径（必填）")
	coverFile := flag.String("cover", "", "封面 PNG 路径（必填）")
	outFile := flag.String("out", "output.png", "输出 PNG 路径")
	flag.Parse()

	if *gameFile == "" || *coverFile == "" {
		flag.Usage()
		os.Exit(1)
	}

	gameJSON, err := os.ReadFile(*gameFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取 game.json 失败: %v\n", err)
		os.Exit(1)
	}

	coverPNG, err := os.ReadFile(*coverFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取封面 PNG 失败: %v\n", err)
		os.Exit(1)
	}

	out, err := injectTextChunk(coverPNG, "gw_game", gameJSON)
	if err != nil {
		fmt.Fprintf(os.Stderr, "注入 chunk 失败: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(*outFile, out, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "写出文件失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("已写出 %s（%d 字节）\n", *outFile, len(out))
}

// injectTextChunk 在 PNG 的 IEND chunk 之前插入一个 tEXt chunk
// value 会被 base64 编码后写入
func injectTextChunk(pngData []byte, keyword string, value []byte) ([]byte, error) {
	const pngSig = "\x89PNG\r\n\x1a\n"
	if len(pngData) < 8 || string(pngData[:8]) != pngSig {
		return nil, fmt.Errorf("不是有效的 PNG 文件")
	}

	encoded := base64.StdEncoding.EncodeToString(value)
	// tEXt chunk data = keyword + 0x00 + value
	chunkData := append([]byte(keyword), 0x00)
	chunkData = append(chunkData, []byte(encoded)...)

	textChunk := makeChunk("tEXt", chunkData)

	// 找到 IEND chunk 的位置，在它之前插入
	iendOffset := findIEND(pngData[8:])
	if iendOffset < 0 {
		return nil, fmt.Errorf("PNG 中未找到 IEND chunk")
	}
	iendOffset += 8 // 加上签名偏移

	var buf bytes.Buffer
	buf.Write(pngData[:iendOffset])
	buf.Write(textChunk)
	buf.Write(pngData[iendOffset:])
	return buf.Bytes(), nil
}

func makeChunk(chunkType string, data []byte) []byte {
	buf := make([]byte, 4+4+len(data)+4)
	binary.BigEndian.PutUint32(buf[0:4], uint32(len(data)))
	copy(buf[4:8], chunkType)
	copy(buf[8:], data)
	crc := crc32.NewIEEE()
	crc.Write(buf[4 : 8+len(data)])
	binary.BigEndian.PutUint32(buf[8+len(data):], crc.Sum32())
	return buf
}

func findIEND(data []byte) int {
	offset := 0
	for offset+8 <= len(data) {
		chunkLen := int(binary.BigEndian.Uint32(data[offset : offset+4]))
		chunkType := string(data[offset+4 : offset+8])
		if chunkType == "IEND" {
			return offset
		}
		offset += 8 + chunkLen + 4
	}
	return -1
}
