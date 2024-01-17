package reporter

import (
	"context"
	"fmt"
	"time"
)

type Reporter struct {
	files     map[string]*ReportItem
	prevSize  int64
	totalSize int64
	prevTime  time.Time
}
type ReportItem struct {
	current, total int64
}

func (r *Reporter) Init(fileName string, total int64) {
	if r.files == nil {
		r.files = make(map[string]*ReportItem)
	}
	r.files[fileName] = &ReportItem{total: total}
}

func (r *Reporter) Increment(fileName string, size int64) {
	r.files[fileName].current += size
	r.totalSize += size
}

func (r *Reporter) Done(fileName string) {
	delete(r.files, fileName)
}

func (r *Reporter) Worker(ctx context.Context) {
	timer := time.NewTicker(1 * time.Second)
	fmt.Printf("\033[2J")
	r.prevTime = time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			now := time.Now()
			r.display(&now)
			r.prevTime = now
			r.prevSize = r.totalSize
		}
	}
}
func (r *Reporter) display(now *time.Time) {
	const second = float64(time.Second)

	fmt.Printf("\033[0;0H")
	deltaTime := float64(now.Sub(r.prevTime)) / second
	deltaSize := float64(r.totalSize - r.prevSize)
	fmt.Printf("Speed: %s/s\n", normalizeSize(int64(deltaSize/deltaTime)))
	for fileName, item := range r.files {
		fmt.Printf("%s: %s/%s\n", fileName, normalizeSize(item.current), normalizeSize(item.total))
	}
}

func normalizeSize(size int64) string {
	const (
		KB = 1 << 10
		MB = 1 << 20
		GB = 1 << 30
		TB = 1 << 40
	)

	switch {
	case size >= TB:
		return fmt.Sprintf("%.2f TB", float64(size)/TB)
	case size >= GB:
		return fmt.Sprintf("%.2f GB", float64(size)/GB)
	case size >= MB:
		return fmt.Sprintf("%.2f MB", float64(size)/MB)
	case size >= KB:
		return fmt.Sprintf("%.2f KB", float64(size)/KB)
	default:
		return fmt.Sprintf("%d bytes", size)
	}
}
