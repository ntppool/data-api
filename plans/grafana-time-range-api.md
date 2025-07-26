# DETAILED IMPLEMENTATION PLAN: Grafana Time Range API with Future Downsampling Support

## Overview
Implement a new Grafana-compatible API endpoint `/api/v2/server/scores/{server}/{mode}` that returns time series data in Grafana format with time range support and future downsampling capabilities.

## API Specification

### Endpoint
- **URL**: `/api/v2/server/scores/{server}/{mode}`
- **Method**: GET
- **Path Parameters**:
  - `server`: Server IP address or ID (same validation as existing API)
  - `mode`: Only `json` supported initially

### Query Parameters (following Grafana conventions)
- `from`: Unix timestamp in seconds (required)
- `to`: Unix timestamp in seconds (required) 
- `maxDataPoints`: Integer, default 50000, max 50000 (for future downsampling)
- `monitor`: Monitor ID, name prefix, or "*" for all (optional, same as existing)
- `interval`: Future downsampling interval like "1m", "5m", "1h" (optional, not implemented initially)

### Response Format
Grafana table format JSON array (more efficient than separate series):
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
      [1753431151000, 20.0, 18.073, -0.000768],
      [1753430063000, 20.0, 18.209, null]
    ]
  }
]
```

## Implementation Details

### 1. Server Routing (`server/server.go`)
Add new route after existing scores routes:
```go
e.GET("/api/v2/server/scores/:server/:mode", srv.scoresTimeRange)
```

**Note**: Initially attempted `:server.:mode` pattern, but Echo router cannot properly parse IP addresses with dots using this pattern. Changed to `:server/:mode` to match existing API pattern and ensure compatibility with IP addresses like `23.155.40.38`.

## Key Implementation Clarifications

### Monitor Filtering Behavior
- **monitor=\***: Return ALL monitors (no monitor count limit)
- **50k datapoint limit**: Applied in database query (LIMIT clause)
- Return whatever data we get from database to user (no post-processing truncation)

### Null Value Handling Strategy
- **Score**: Always include (should never be null)
- **RTT**: Skip datapoints where RTT is null
- **Offset**: Skip datapoints where offset is null

### Time Range Validation Rules
- **Zero duration**: Return 400 Bad Request
- **Future timestamps**: Allow for now
- **Minimum range**: 1 second
- **Maximum range**: 90 days

### 2. New Handler Function (`server/grafana.go`)

#### Function Signature
```go
func (srv *Server) scoresTimeRange(c echo.Context) error
```

#### Parameter Parsing & Validation
```go
// Extend existing historyParameters struct for time range support
type timeRangeParams struct {
    historyParameters // embed existing struct
    from              time.Time  
    to                time.Time
    maxDataPoints     int
    interval          string // for future downsampling
}

func (srv *Server) parseTimeRangeParams(ctx context.Context, c echo.Context) (timeRangeParams, error) {
    // Start with existing parameter parsing logic
    baseParams, err := srv.getHistoryParameters(ctx, c)
    if err != nil {
        return timeRangeParams{}, err
    }
    
    // Parse and validate from/to second timestamps
    // Validate time range (max 90 days, min 1 second)
    // Parse maxDataPoints (default 50000, max 50000)
    // Return extended parameters
}
```

#### Response Structure
```go
type ColumnDef struct {
    Text string `json:"text"`
    Type string `json:"type"`
    Unit string `json:"unit,omitempty"`
}

type GrafanaTableSeries struct {
    Target  string            `json:"target"`
    Tags    map[string]string `json:"tags"`
    Columns []ColumnDef       `json:"columns"`
    Values  [][]interface{}   `json:"values"`
}

type GrafanaTimeSeriesResponse []GrafanaTableSeries
```

#### Cache Control
```go
// Reuse existing setHistoryCacheControl function for consistency
// Logic based on data recency and entry count:
// - Empty or >8h old data: "s-maxage=260,max-age=360"
// - Single entry: "s-maxage=60,max-age=35" 
// - Multiple entries: "s-maxage=90,max-age=120"
setHistoryCacheControl(c, history)
```

### 3. ClickHouse Data Access (`chdb/logscores.go`)

#### New Method
```go
func (d *ClickHouse) LogscoresTimeRange(ctx context.Context, serverID, monitorID int, from, to time.Time, limit int) ([]ntpdb.LogScore, error) {
    // Build query with time range WHERE clause
    // Always order by ts ASC (Grafana convention)
    // Apply limit to prevent memory issues
    // Use same row scanning logic as existing Logscores method
}
```

#### Query Structure
```sql
SELECT id, monitor_id, server_id, ts,
       toFloat64(score), toFloat64(step), offset,
       rtt, leap, warning, error
FROM log_scores  
WHERE server_id = ?
  AND ts >= ?
  AND ts <= ?
  [AND monitor_id = ?]  -- if specific monitor requested
ORDER BY ts ASC
LIMIT ?
```

### 4. Data Transformation Logic (`server/grafana.go`)

#### Core Transformation Function
```go
func transformToGrafanaTableFormat(history *logscores.LogScoreHistory, monitors []ntpdb.Monitor) GrafanaTimeSeriesResponse {
    // Group data by monitor_id (one series per monitor)
    // Create table format with columns: time, score, rtt, offset
    // Convert timestamps to milliseconds
    // Build proper target names and tags
    // Handle null values appropriately in table values
}
```

#### Grouping Strategy
1. **Group by Monitor**: One table series per monitor
2. **Table Columns**: time, score, rtt, offset (all metrics in one table)
3. **Target Naming**: `monitor{name={sanitized_monitor_name}}`
4. **Tag Structure**: Include monitor metadata (no metric type needed)
5. **Monitor Status**: Query real monitor data using `q.GetServerScores()` like existing API
6. **Series Ordering**: No guaranteed order (standard Grafana behavior)
7. **Efficiency**: More efficient than separate series - less JSON overhead

#### Timestamp Conversion
```go
timestampMs := logScore.Ts.Unix() * 1000
```

### 5. Error Handling

#### Validation Errors (400 Bad Request)
- Invalid timestamp format
- from >= to (including zero duration)
- Time range too large (> 90 days)
- Time range too small (< 1 second minimum)
- maxDataPoints > 50000
- Invalid mode (not "json")

#### Not Found Errors (404)
- Server not found
- Monitor not found  
- Server deleted

#### Server Errors (500)
- ClickHouse connection issues
- Database query errors

### 6. Future Downsampling Design

#### API Extension Points
- `interval` parameter parsing ready
- `maxDataPoints` limit already enforced
- Response format supports downsampled data seamlessly

#### Downsampling Algorithm (Future Implementation)
```go
// When datapoints > maxDataPoints:
// 1. Calculate downsample interval: (to - from) / maxDataPoints
// 2. Group data into time buckets  
// 3. Aggregate per bucket: avg for score/rtt, last for offset
// 4. Return aggregated datapoints
```

## Testing Strategy

### Unit Tests
- Parameter parsing and validation
- Data transformation logic
- Error handling scenarios
- Timestamp conversion accuracy

### Integration Tests  
- End-to-end API requests
- ClickHouse query execution
- Multiple monitor scenarios
- Large time range handling

### Manual Testing
- Grafana integration testing
- Performance with various time ranges
- Cache behavior validation

## Performance Considerations

### Current Implementation
- 50k datapoint limit applied in database query (LIMIT clause) (covers ~few weeks of data)
- ClickHouse-only for better range query performance
- Proper indexing on (server_id, ts) assumed
- Table format more efficient than separate time series (less JSON overhead)

### Future Optimizations (Critical for Production)
- **Downsampling for large ranges**: Essential for 90-day queries with reasonable performance
- Query optimization based on range size
- Potential parallel monitor queries
- Adaptive sampling rates based on time range duration

## Documentation Updates

### API.md Addition
```markdown
### 7. Server Scores Time Range (v2)

**GET** `/api/v2/server/scores/{server}/{mode}`

Grafana-compatible time series endpoint for NTP server scoring data.

#### Path Parameters
- `server`: Server IP address or ID
- `mode`: Response format (`json` only)

#### Query Parameters  
- `from`: Start time as Unix timestamp in seconds (required)
- `to`: End time as Unix timestamp in seconds (required)
- `maxDataPoints`: Maximum data points to return (default: 50000, max: 50000)
- `monitor`: Monitor filter (ID, name prefix, or "*" for all)

#### Response Format
Grafana table format array with one series per monitor containing all metrics as columns.
```

## Key Research Findings

### Grafana Error Format Requirements
- **HTTP Status Codes**: Standard 400/404/500 work fine
- **Response Body**: JSON preferred with `Content-Type: application/json`
- **Structure**: Simple `{"error": "message", "status": code}` is sufficient
- **Compatibility**: Existing Echo error patterns are Grafana-compatible

### Data Volume Considerations
- **50k Datapoint Limit**: Only covers ~few weeks of data, not sufficient for 90-day ranges
- **Downsampling Critical**: Required for production use with 90-day time ranges
- **Current Approach**: Acceptable for MVP, downsampling essential for full utility

## Implementation Checklist

### Phase 0: Grafana Table Format Validation ✅ **COMPLETED**
- [x] Add test endpoint `/api/v2/test/grafana-table` returning sample table format
- [x] Implement Grafana table format response structures in `server/grafana.go`
- [x] Add structured logging and OpenTelemetry tracing to test endpoint
- [x] Verify endpoint compiles and serves correct JSON format
- [x] Test endpoint response format and headers (CORS, Content-Type, Cache-Control)
- [ ] Test with actual Grafana instance to validate table format compatibility
- [ ] Confirm time series panels render table format correctly
- [ ] Validate column types and units display properly

#### Phase 0 Implementation Details
**Files Created/Modified:**
- `server/grafana.go`: New file containing Grafana table format structures and test endpoint
- `server/server.go`: Added route `e.GET("/api/v2/test/grafana-table", srv.testGrafanaTable)`

**Test Endpoint Features:**
- **URL**: `http://localhost:8030/api/v2/test/grafana-table`
- **Response Format**: Grafana table format with realistic NTP Pool data
- **Sample Data**: Two monitor series (zakim1-yfhw4a, nj2-mon01) with time-based values
- **Columns**: time, score, rtt (ms), offset (s) with proper units
- **Null Handling**: Demonstrates null offset values
- **Headers**: CORS, JSON content-type, cache control
- **Observability**: Structured logging with context, OpenTelemetry tracing

**Recommended Grafana Data Source**: JSON API plugin (`marcusolsson-json-datasource`) - ideal for REST APIs returning table format JSON

### Phase 1: Core Implementation ✅ **COMPLETED**
- [x] Add route in server.go (fixed routing pattern from `:server.:mode` to `:server/:mode`)
- [x] Implement parseTimeRangeParams function for parameter validation
- [x] Add LogscoresTimeRange method to ClickHouse with time range filtering
- [x] Implement transformToGrafanaTableFormat function with monitor grouping
- [x] Add scoresTimeRange handler with full error handling
- [x] Error handling and validation (reuse existing Echo patterns)
- [x] Cache control headers (reuse setHistoryCacheControl)

#### Phase 1 Implementation Details
**Key Components Built:**
- **Route Pattern**: `/api/v2/server/scores/:server/:mode` (matches existing API consistency)
- **Parameter Validation**: Full validation of `from`/`to` timestamps, `maxDataPoints`, time ranges
- **ClickHouse Integration**: `LogscoresTimeRange()` with time-based WHERE clauses and ASC ordering
- **Data Transformation**: Grafana table format with monitor grouping and null value handling
- **Complete Handler**: `scoresTimeRange()` with server validation, error handling, caching, and CORS

**Routing Fix**: Changed from `:server.:mode` to `:server/:mode` to resolve Echo router issue with IP addresses containing dots (e.g., `23.155.40.38`).

**Files Created/Modified in Phase 1:**
- `server/grafana.go`: Complete implementation with all structures and functions
  - `timeRangeParams` struct and `parseTimeRangeParams()` function
  - `transformToGrafanaTableFormat()` function with monitor grouping
  - `scoresTimeRange()` handler with full error handling
  - `sanitizeMonitorName()` utility function
- `server/server.go`: Added route `e.GET("/api/v2/server/scores/:server/:mode", srv.scoresTimeRange)`
- `chdb/logscores.go`: Added `LogscoresTimeRange()` method for time-based queries

**Production Testing Results** (July 25, 2025):
- ✅ **Real Data Verification**: Successfully tested with server `102.64.112.164` over 12-hour time range
- ✅ **Multiple Monitor Support**: Returns data for multiple monitors (`defra1-210hw9t`, `recentmedian`)
- ✅ **Data Quality Validation**: 
  - RTT conversion (microseconds → milliseconds): ✅ Working
  - Timestamp conversion (seconds → milliseconds): ✅ Working  
  - Null value handling: ✅ Working (recentmedian has null RTT/offset as expected)
  - Monitor grouping: ✅ Working (one series per monitor)
- ✅ **API Parameter Changes**: Successfully changed from milliseconds to seconds for user-friendliness
- ✅ **Volume Testing**: Handles 100+ data points per monitor efficiently
- ✅ **Error Handling**: All validation working (400 for invalid params, 404 for missing servers)
- ✅ **Performance**: Sub-second response times for 12-hour ranges

**Sample Working Request:**
```bash
curl 'http://localhost:8030/api/v2/server/scores/102.64.112.164/json?from=1753457764&to=1753500964&monitor=*'
```

### Phase 2: Testing & Polish
- [ ] Unit tests for all functions
- [ ] Integration tests
- [ ] Manual Grafana testing with real data
- [ ] Performance testing with large ranges (up to 50k points)
- [ ] API documentation updates

### Phase 3: Future Enhancement Ready
- [ ] Interval parameter parsing (no-op initially)
- [ ] Downsampling framework hooks (critical for 90-day ranges)
- [ ] Monitoring and metrics for new endpoint

This design provides a solid foundation for immediate Grafana integration while being fully prepared for future downsampling capabilities without breaking changes.

## Critical Notes for Production

- **Downsampling Required**: 50k datapoint limit means 90-day ranges will hit limits quickly
- **Table Format Validation**: Phase 0 testing ensures Grafana compatibility before full implementation
- **Error Handling**: Existing Echo patterns are sufficient for Grafana requirements
- **Scalability**: Current design handles weeks of data well, downsampling needed for months