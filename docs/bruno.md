# APEX - Bruno Collection

The `bruno/` directory contains a complete Bruno collection covering all 70+ iRacing API endpoints. It's useful for low-level exploration, scripted testing, or cases where you want direct access to raw API responses before S3 fetching.

## Setup

1. Install [Bruno](https://www.usebruno.com) (free, open source)
2. In Bruno, choose **Open Collection** and select the `bruno/` folder
3. Click **Send** on any endpoint — Bruno handles OAuth automatically on first run (opens your browser)

## How S3 Data Handling Works

Most iRacing endpoints don't return data directly — they return a signed S3 link. Bruno's post-response scripts detect the response type and store the relevant variables automatically.

| Response type | Pattern | How to get data |
|---|---|---|
| Simple S3 | `{link: "...", expires: "..."}` | Run **Fetch-S3-Data** |
| Chunked | `{data: {chunk_info: {...}}}` | Run **Fetch-S3-Data**, adjust `s3_chunk_index` for subsequent chunks |
| Direct | Raw JSON | Already in the response body — no extra step needed |

### Simple S3 flow
```
Run endpoint → response contains S3 link → Run Fetch-S3-Data → view data
```

### Chunked data flow
```
Run search endpoint → response contains chunk info → Run Fetch-S3-Data (chunk 1)
→ set s3_chunk_index to 1 → Run Fetch-S3-Data (chunk 2) → repeat as needed
```

## Helper Requests

### Fetch-S3-Data
Universal S3 data fetcher — works for both simple links and chunked responses.
- **Simple**: fetches the stored S3 link
- **Chunked**: fetches the chunk at the current `s3_chunk_index`

To fetch a specific chunk:
1. Open Bruno's Environment panel
2. Set `s3_chunk_index` to the desired chunk number (0-based)
3. Run **Fetch-S3-Data**

### Chunk-Navigator
Shows all available chunks for the last chunked response and your current position.

## Environment Variables

Bruno's post-response scripts manage these automatically after each request:

| Variable | Description |
|---|---|
| `s3_link` | Current S3 URL or first chunk URL |
| `s3_type` | `simple`, `chunked`, or `direct` |
| `s3_expires` | Link expiration time |
| `s3_chunk_count` | Total number of chunks |
| `s3_chunk_index` | Current chunk (0-based) |
| `s3_base_url` | Base URL for chunk files |
| `s3_chunk_files` | Array of chunk filenames |

## Endpoint Categories

| Folder | Contents |
|---|---|
| 02-Member | Profile, account info, login history |
| 03-Cars | Vehicle specs and assets |
| 04-Tracks | Track details and layouts |
| 05-Stats | Career stats, season stats, member stats |
| 06-Results | Race results, subsession details |
| 07-Series | Series information |
| 08-Leagues | League information and standings |
| 09-Lookup | Helmets, paint patterns, sponsors, avatars |
| 10-Constants | Static reference data |
| 11-Documentation | API schema and documentation |
| 12-DriverStats | Driver-specific statistics |
| 13-Hosted | Hosted session results |
| 14-Season | Season schedule and details |
| 15-Session | Event sessions and schedules |
| 16-Team | Team data and standings |
| 17-TimeAttack | Time attack event data |

## Troubleshooting

**"No S3 link found in environment"** — run an API endpoint first; the post-response script populates the variable.

**S3 link expired (404)** — links expire in ~2 minutes; re-run the original endpoint to get a fresh one, then immediately run Fetch-S3-Data.

**Chunk index out of range** — `s3_chunk_index` is 0-based. Run Chunk-Navigator to see how many chunks are available.

**Authentication issues** — tokens refresh automatically. If stuck, clear Bruno's environment variables or re-run any endpoint to trigger a fresh OAuth flow.
