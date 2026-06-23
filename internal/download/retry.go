package download

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type httpClient struct {
	client  *http.Client
	retries int
	baseWait time.Duration
	maxWait  time.Duration
}

func defaultClient() *httpClient {
	return &httpClient{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		retries:  5,
		baseWait: 1 * time.Second,
		maxWait:  30 * time.Second,
	}
}

func (c *httpClient) getBytes(url string) ([]byte, error) {
	var lastErr error
	for i := 0; i <= c.retries; i++ {
		resp, err := c.client.Get(url)
		if err != nil {
			lastErr = err
			c.wait(i)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
			c.wait(i)
			continue
		}

		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			c.wait(i)
			continue
		}

		return data, nil
	}
	return nil, fmt.Errorf("get %s after %d retries: %w", url, c.retries, lastErr)
}

func (c *httpClient) downloadFile(destPath, url, expectedSHA string) error {
	dir := filepath.Dir(destPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	var lastErr error
	for i := 0; i <= c.retries; i++ {
		if err := c.doDownload(destPath, url, expectedSHA); err != nil {
			lastErr = err
			os.Remove(destPath)
			c.wait(i)
			continue
		}
		return nil
	}
	return fmt.Errorf("download %s after %d retries: %w", url, c.retries, lastErr)
}

func (c *httpClient) doDownload(destPath, url, expectedSHA string) error {
	resp, err := c.client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	outFile, err := os.Create(destPath + ".tmp")
	if err != nil {
		return err
	}

	h := sha1.New()
	writer := io.MultiWriter(outFile, h)

	if _, err := io.Copy(writer, resp.Body); err != nil {
		outFile.Close()
		os.Remove(destPath + ".tmp")
		return fmt.Errorf("write: %w", err)
	}

	outFile.Close()

	gotSHA := hex.EncodeToString(h.Sum(nil))
	if expectedSHA != "" && gotSHA != expectedSHA {
		os.Remove(destPath + ".tmp")
		return fmt.Errorf("SHA1 mismatch: expected %s, got %s", expectedSHA, gotSHA)
	}

	if err := os.Rename(destPath+".tmp", destPath); err != nil {
		return err
	}

	return nil
}

func (c *httpClient) wait(attempt int) {
	if attempt >= c.retries {
		return
	}
	delay := float64(c.baseWait) * math.Pow(2, float64(attempt))
	if delay > float64(c.maxWait) {
		delay = float64(c.maxWait)
	}
	time.Sleep(time.Duration(delay))
}

func sha1Hex(data []byte) string {
	h := sha1.Sum(data)
	return hex.EncodeToString(h[:])
}
