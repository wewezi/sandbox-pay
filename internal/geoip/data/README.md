# Offline IP ownership data

Source: [ip2region](https://github.com/lionsoul2014/ip2region/tree/6a276cc7c4f9f2d7ff1d8ace14b91af59752f943), pinned commit `6a276cc7c4f9f2d7ff1d8ace14b91af59752f943` (2026-10-28).

The original `data/ip2region_v4.xdb` and `data/ip2region_v6.xdb` files are gzip-compressed here and embedded in the Go binary. The source license is preserved in [LICENSE.ip2region.md](LICENSE.ip2region.md). The Go search SDK is Apache-2.0, with its license preserved in [LICENSE.ip2region-sdk.md](LICENSE.ip2region-sdk.md), and is recorded in `go.mod`.

| Database | Uncompressed bytes | SHA-256 of original XDB |
|---|---:|---|
| IPv4 | 11,122,714 | `46249ad1b33bcaa4cc37475b93746cec2b00acffc3316cc4f56167ac821ea2ad` |
| IPv6 | 37,277,813 | `939f6b46bd2b8bec3cf7c5ceb8ba782266ae9b1f35b5ba7916700dec0b7506ed` |

No lookup sends order IPs to an external service. Data is decompressed once per address family, on the first public-address lookup, and shared across locator instances. Results describe IP network ownership, not a person's physical location.

To update, download both XDB files from a reviewed source commit, gzip them, update the expected sizes and SHA-256 values in `geoip.go` and this file, and run `go test ./internal/geoip`. Keep the source license and provenance with the new data.
