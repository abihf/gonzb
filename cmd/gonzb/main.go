package main

import (
	"fmt"
	"os"

	"github.com/abihf/gonzb/internal/nzb"
)

func main() {
	file, _ := os.Open("test_download_1000MB.nzb")
	defer file.Close()
	n, _ := nzb.Parse(file)
	fmt.Printf("Nzb %v\n", n.Files[0].Segments[0].ID)
}
