# Video Downloader

A high-performance video downloader that supports downloading videos from various websites, even where direct downloads are not provided. It downloads videos in chunks using a multithreaded approach for faster performance (for chunked video).

## Features
- Supports video downloading from multiple websites
- Chunk-based downloading for better stability
- Multithreaded execution for faster downloads
- Automatic URL detection and processing
- Provides a direct video URL if chunked downloading is not required

## Usage
```sh
./downloader -url <video_page_url> -segurl <segment_url> -threads <num_threads> -start <start_chunk>
```

### Example
```sh
./downloader -url "https://example.com/video123"
```
### Example with direct url with chunks
```sh
./downloader -segurl "https://example.com/video123-{seg}-1.ts" -threads 10 -start 1
```

### It can also display matching direct links if an error occurs and/or it cannot find the video itself.
```sh
./downloader -url "https://example.com/video123" -show
```

## Requirements
- Go 1.18+
- `go-session-client` library

## Installation
```sh
go build -o downloader main.go
```

## Notes
- If the video is chunked, it will download in segments.
- If the video is not chunked, it will return the direct video URL.
- The tool handles rate limits and retries failed requests automatically.

