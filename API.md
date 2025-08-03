# NTP Pool Data API Documentation

This document describes the REST API endpoints provided by the NTP Pool data API server.

## Base URL

The API server runs on port 8030. All endpoints are accessible at:
- Production: `https://www.ntppool.org/api/...`
- Local development: `http://localhost:8030/api/...`

## Common Response Headers

All API responses include:
- `Server`: Version information (e.g., `data-api/1.2.3+abc123`)
- `Cache-Control`: Caching directives
- `Access-Control-Allow-Origin`: CORS configuration

## Endpoints

### 1. User Country Data

**GET** `/api/usercc`

Returns DNS query statistics by user country code and NTP pool zone statistics.

#### Response Format
```json
{
  "UserCountry": [
    {
      "CC": "us",
      "IPv4": 42.5,
      "IPv6": 12.3
    }
  ],
  "ZoneStats": {
    "zones": [
      {
        "zone_name": "us",
        "netspeed_active": 1000,
        "server_count": 450
      }
    ]
  }
}
```

#### Response Fields
- `UserCountry`: Array of country statistics
  - `CC`: Two-letter country code
  - `IPv4`: IPv4 query percentage
  - `IPv6`: IPv6 query percentage
- `ZoneStats`: NTP pool zone information

#### Cache Control
- `Cache-Control`: Varies based on data freshness

---

### 2. DNS Query Counts

**GET** `/api/dns/counts`

Returns aggregated DNS query counts from ClickHouse analytics.

#### Response Format
```json
{
  "total_queries": 1234567,
  "by_country": {
    "us": 456789,
    "de": 234567
  },
  "by_query_type": {
    "A": 987654,
    "AAAA": 345678
  }
}
```

#### Cache Control
- `Cache-Control`: `s-maxage=30,max-age=60`

---

### 3. Server DNS Answers

**GET** `/api/server/dns/answers/{server}`

Returns DNS answer statistics for a specific NTP server, including geographic distribution and scoring metrics.

#### Path Parameters
- `server`: Server IP address (IPv4 or IPv6)

#### Response Format
```json
{
  "Server": [
    {
      "CC": "us",
      "Count": 12345,
      "Points": 1234.5,
      "Netspeed": 567.8
    }
  ],
  "PointSymbol": "‱"
}
```

#### Response Fields
- `Server`: Array of country-specific statistics
  - `CC`: Country code where DNS queries originated
  - `Count`: Number of DNS answers served
  - `Points`: Calculated scoring points (basis: 10,000)
  - `Netspeed`: Network speed score relative to zone capacity
- `PointSymbol`: Symbol used for point calculations ("‱" = per 10,000)

#### Error Responses
- `400 Bad Request`: Invalid server IP format
- `404 Not Found`: Server not found
- `500 Internal Server Error`: Database error

#### Cache Control
- Success: `public,max-age=1800`
- Errors: `public,max-age=300`

#### URL Canonicalization
Redirects to canonical IP format with `308 Permanent Redirect` if:
- IP format is not canonical
- Query parameters are present

---

### 4. Server Score History (Legacy)

**GET** `/api/server/scores/{server}/{mode}`

**⚠️ Legacy API** - Returns historical scoring data for an NTP server in JSON or CSV format. For enhanced features and higher limits, use the [v2 API](#7-server-score-history-v2---enhanced-time-range-api) instead.

#### Path Parameters
- `server`: Server IP address or ID
- `mode`: Response format (`json` or `log`)

#### Query Parameters
- `limit`: Maximum number of records (default: 100, max: 10000)
- `monitor`: Monitor ID or name prefix (default: "recentmedian.scores.ntp.dev")
  - Use `*` for all monitors
  - Use monitor ID number
  - Use monitor name prefix (e.g., "recentmedian")
- `since`: Unix timestamp for start time
- `source`: Data source (`m` for MySQL, `c` for ClickHouse)
- `full_history`: Include full history (private IPs only)

#### JSON Response Format (`mode=json`)
```json
{
  "history": [
    {
      "ts": 1640995200,
      "offset": 0.001234,
      "step": 0.5,
      "score": 20.0,
      "monitor_id": 123,
      "rtt": 45.6
    }
  ],
  "monitors": [
    {
      "id": 123,
      "name": "recentmedian.scores.ntp.dev",
      "type": "ntp",
      "ts": "2022-01-01T12:00:00Z",
      "score": 19.5,
      "status": "active",
      "avg_rtt": 45.2
    }
  ],
  "server": {
    "ip": "192.0.2.1"
  }
}
```

#### CSV Response Format (`mode=log`)
Returns CSV data with headers:
```
ts_epoch,ts,offset,step,score,monitor_id,monitor_name,rtt,leap,error
1640995200,2022-01-01 12:00:00,0.001234,0.5,20.0,123,recentmedian.scores.ntp.dev,45.6,,
```

#### CSV Fields
- `ts_epoch`: Unix timestamp
- `ts`: Human-readable timestamp
- `offset`: Time offset in seconds
- `step`: NTP step value
- `score`: Computed score
- `monitor_id`: Monitor identifier
- `monitor_name`: Monitor display name
- `rtt`: Round-trip time in milliseconds
- `leap`: Leap second indicator
- `error`: Error message (sanitized for CSV)

#### Error Responses
- `404 Not Found`: Invalid mode, server not found, or monitor not found
- `500 Internal Server Error`: Database error

#### Cache Control
Dynamic based on data freshness:
- Recent data: `s-maxage=90,max-age=120`
- Older data: `s-maxage=260,max-age=360`

---

### 5. Zone Counts

**GET** `/api/zone/counts/{zone_name}`

Returns historical server count and network capacity data for an NTP pool zone.

#### Path Parameters
- `zone_name`: Zone name (e.g., "us", "europe", "@" for global)

#### Query Parameters
- `limit`: Maximum number of date entries to return

#### Response Format
```json
{
  "history": [
    {
      "d": "2022-01-01",
      "ts": 1640995200,
      "rc": 450,
      "ac": 380,
      "w": 12500,
      "iv": "v4"
    }
  ]
}
```

#### Response Fields
- `history`: Array of historical data points
  - `d`: Date in YYYY-MM-DD format
  - `ts`: Unix timestamp
  - `rc`: Registered server count
  - `ac`: Active server count
  - `w`: Network capacity (netspeed active)
  - `iv`: IP version ("v4" or "v6")

#### Data Sampling
When `limit` is specified, the API intelligently samples data points to provide representative historical coverage while staying within the limit.

#### Error Responses
- `404 Not Found`: Zone not found
- `500 Internal Server Error`: Database error

#### Cache Control
- `s-maxage=28800, max-age=7200`

---

### 6. Graph Images

**GET** `/graph/{server}/{type}`

Returns generated graph images for server visualization.

#### Path Parameters
- `server`: Server IP address
- `type`: Graph type (currently only "offset.png" supported)

#### Response
- **Content-Type**: `image/png` or upstream service content type
- **Body**: Binary image data

#### Features
- Canonical URL enforcement (redirects if server IP format is non-canonical)
- Query parameter removal (redirects to clean URLs)
- Upstream service integration via HTTP proxy

#### Error Responses
- `404 Not Found`: Invalid image type or server not found
- `500 Internal Server Error`: Upstream service error

#### Cache Control
- Success: `public,max-age=1800,s-maxage=1350`
- Errors: `public,max-age=240`

---

### 7. Server Score History (v2) - Enhanced Time Range API

**GET** `/api/v2/server/scores/{server}/{mode}`

**🆕 Recommended API** - Returns historical scoring data for an NTP server in Grafana-compatible table format with enhanced time range support and relative time expressions.

#### Path Parameters
- `server`: Server IP address or ID
- `mode`: Response format (`json` only)

#### Query Parameters
- `from`: Start time (required) - Unix timestamp or relative time (e.g., "-3d", "-2h", "-30m")
- `to`: End time (required) - Unix timestamp or relative time (e.g., "-1d", "-1h", "0s")
- `maxDataPoints`: Maximum data points to return (default: 50000, max: 50000)
- `monitor`: Monitor filter (ID, name prefix, or "*" for all monitors)
- `interval`: Future downsampling interval (not implemented)

#### Time Format Support
The v2 API supports both Unix timestamps and relative time expressions:

**Unix Timestamps:**
- `from=1753500964&to=1753587364` - Standard Unix seconds

**Relative Time Expressions:**
- `from=-3d&to=-1d` - From 3 days ago to 1 day ago
- `from=-2h&to=-30m` - From 2 hours ago to 30 minutes ago
- `from=-1d&to=0s` - From 1 day ago to now

**Supported Units:**
- `s` - seconds
- `m` - minutes  
- `h` - hours
- `d` - days

**Format:** `[-]<number><unit>` (negative sign for past, no sign for future)

#### Response Format
Grafana table format optimized for visualization:

```json
[
  {
    "target": "monitor{name=zakim1-yfhw4a}",
    "tags": {
      "monitor_id": "126",
      "monitor_name": "zakim1-yfhw4a", 
      "type": "monitor",
      "status": "active"
    },
    "columns": [
      {"text": "time", "type": "time"},
      {"text": "score", "type": "number"},
      {"text": "rtt", "type": "number", "unit": "ms"},
      {"text": "offset", "type": "number", "unit": "s"}
    ],
    "values": [
      [1753431667000, 20.0, 18.865, -0.000267],
      [1753431419000, 20.0, 18.96, -0.000390],
      [1753431151000, 20.0, 18.073, -0.000768]
    ]
  }
]
```

#### Response Structure
- **One series per monitor**: Efficient grouping by monitor ID
- **Table format**: All metrics (time, score, rtt, offset) in columns
- **Timestamps**: Converted to milliseconds for Grafana compatibility
- **Null handling**: Null RTT/offset values preserved as `null`

#### Limits and Constraints
- **Data points**: Maximum 50,000 records per request
- **Time range**: Maximum 90 days per request  
- **Minimum range**: 1 second
- **Data source**: ClickHouse only (for better time range performance)

#### Example Requests

**Recent data with relative times:**
```
GET /api/v2/server/scores/192.0.2.1/json?from=-3d&to=-1h&monitor=*
```

**Specific time range:**
```
GET /api/v2/server/scores/192.0.2.1/json?from=1753500000&to=1753586400&monitor=recentmedian
```

**All monitors, last 24 hours:**
```
GET /api/v2/server/scores/192.0.2.1/json?from=-1d&to=0s&monitor=*&maxDataPoints=10000
```

#### Error Responses
- `400 Bad Request`: Invalid time format, range too large/small, or invalid parameters
- `404 Not Found`: Server not found, invalid mode, or monitor not found  
- `500 Internal Server Error`: Database or internal error

#### Cache Control
Dynamic caching based on data characteristics:
- Recent data: `s-maxage=90,max-age=120`
- Older data: `s-maxage=260,max-age=360`
- Empty results: `s-maxage=260,max-age=360`

#### Comparison with Legacy API
The v2 API offers significant improvements over `/api/server/scores/{server}/{mode}`:

| Feature | Legacy API | v2 API |
|---------|------------|--------|
| **Record limit** | 10,000 | 50,000 |
| **Time format** | Unix timestamps only | Unix timestamps + relative time |
| **Response format** | Legacy JSON/CSV | Grafana table format |
| **Time range** | Limited by `since` parameter | Full `from`/`to` range support |
| **Maximum range** | No explicit limit | 90 days |
| **Performance** | MySQL + ClickHouse | ClickHouse optimized |

#### Migration Guide
To migrate from legacy API to v2:

**Legacy:**
```
/api/server/scores/192.0.2.1/json?limit=10000&since=1753500000&monitor=*
```

**V2 equivalent:**
```
/api/v2/server/scores/192.0.2.1/json?from=1753500000&to=0s&monitor=*&maxDataPoints=10000
```

**V2 with relative time:**
```
/api/v2/server/scores/192.0.2.1/json?from=-3d&to=-1h&monitor=*
```

---

## Health Check Endpoints

### Health Check
**GET** `:9019/health`

Returns server health status by testing database connections.

#### Query Parameters
- `reset`: Boolean to reset database connection pool

#### Response
- `200 OK`: "ok" - All systems healthy
- `503 Service Unavailable`: "db ping err" - Database connectivity issues

### Metrics
**GET** `:9020/metrics`

Prometheus metrics endpoint for monitoring and observability.

---

## Error Handling

### Standard HTTP Status Codes
- `200 OK`: Successful request
- `308 Permanent Redirect`: URL canonicalization
- `400 Bad Request`: Invalid request parameters
- `404 Not Found`: Resource not found
- `500 Internal Server Error`: Server-side error
- `503 Service Unavailable`: Service temporarily unavailable

### Error Response Format
Most endpoints return plain text error messages for non-2xx responses. Some endpoints may return JSON error objects.

---

## Data Sources

The API integrates multiple data sources:
- **MySQL**: Operational data (servers, zones, accounts, current scores)
- **ClickHouse**: Analytics data (DNS query logs, historical scoring data)

Different endpoints may use different data sources, and some endpoints allow source selection via query parameters.

---

## Rate Limiting and Caching

The API implements extensive caching at multiple levels:
- **Response-level caching**: Each endpoint sets appropriate `Cache-Control` headers
- **Database query optimization**: Efficient queries with proper indexing
- **CDN integration**: Headers configured for CDN caching

Cache durations vary by endpoint and data freshness, ranging from 30 seconds for real-time data to 8 hours for historical data.