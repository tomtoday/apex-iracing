# APEX - API Explorer for iRacing

In 2025, the old method for authenticating to the iRacing Data API using your username and password was phased out in favor of a more secure oAuth based method. You can still use a [password limited flow](https://oauth.iracing.com/oauth2/book/password_limited_flow.html) but it requires you to pre=register a client and you can still expose your credentials if you are not careful. This makes it hard to explore the iRacing Data API.

APEX provides secure, [authenticated access](https://oauth.iracing.com/oauth2/book/authorization_code_flow.html) to the iRacing Data API. It's a developer-friendly tool for exploring and testing iRacing data, with 70+ endpoints organized into 16 logical categories and intelligent S3 data handling. 

APEX is available in two interfaces:
- **Bruno Collection** (`/bruno/`) - For API exploration with the [Bruno](https://www.usebruno.com) client
- **Web Interface** - Simple, browser-based interface for easier data fetching (coming soon)

**Please note:** this tool was created using GitHub Copilot based on the documented endpoints at https://members-ng.iracing.com/data/doc

- **OAuth 2.1 with PKCE**: Secure public client authentication without exposing secrets
- **70+ Endpoints**: Complete API coverage organized into logical categories
- **Intelligent S3 Handling**: Automatic detection and processing of:
  - Simple S3 links (most endpoints)
  - Chunked data responses (search results, large datasets)
  - Direct API responses
- **User-Friendly Output**: Clear console feedback with next steps after each call
- **Easy Data Fetching**: Pre-configured "Fetch-S3-Data" request for seamless data retrieval
- **Chunk Navigation**: Built-in tools for working with multi-chunk responses

## 🔐 Authentication

APEX uses **OAuth 2.1 Authorization Code Flow with PKCE**, which is the recommended approach for public/shared applications. A client ID for APEX has been registered with iRacing already.

### How It Works

1. **First Request**: When you run your first API request, Bruno opens your browser to iRacing's official login page
2. **User Login**: You authenticate directly with iRacing (no third-party credential sharing)
3. **Token Grant**: iRacing grants access tokens that are stored locally in Bruno
4. **Automatic Refresh**: Tokens are automatically refreshed when they expire
5. **Secure**: PKCE prevents authorization code interception attacks

### Public Client Architecture

APEX uses a **public client model**, which means:
- ✅ All users share the same `client_id`: `apex-api-explorer`
- ✅ No `client_secret` is needed (it's registered as a public client)
- ✅ Each user authenticates individually with their own iRacing account
- ✅ Tokens are stored locally and never shared
- ✅ Collection is safe to share on GitHub with no security concerns

### Registered with iRacing

- **Client Name**: APEX - API Explorer for iRacing
- **Client Type**: Public client (desktop/CLI application)
- **Redirect URI**: `http://localhost:3000/callback`
- **Scope**: `iracing.auth`

## 🚀 Getting Started

### Using Bruno Collection

1. **Open in Bruno**: Clone this repo, then open the `bruno/` directory in Bruno
2. **Select an Endpoint**: Choose any endpoint from the 16 categories (e.g., "Get Member Info")
3. **Run Request**: Click Send - Bruno will handle OAuth automatically
4. **Get S3 Link**: The response includes an S3 link (or chunked data info)
5. **Fetch Data**: Run the "Fetch-S3-Data" request to download the actual data

### Using Web Interface (Coming Soon)

1. **Start the server**: `python app.py`
2. **Open browser**: Navigate to `http://127.0.0.1:3000`
3. **Login**: Click "Login" to authenticate with iRacing
4. **Select endpoint**: Choose Member Info, Car List, or Search Series
5. **View data**: Results with automatic S3 fetching appear immediately

## 📁 Project Structure

```
apex-iracing/
├── bruno/                      # Bruno API collection (70+ endpoints)
│   ├── collection.bru          # Main collection with OAuth setup
│   ├── Fetch-S3-Data.bru       # Helper: fetch S3 data
│   ├── Chunk-Navigator.bru     # Helper: browse chunked data
│   ├── bruno.json              # Bruno collection metadata
│   ├── 02-Member/              # User profile and account data
│   ├── 03-Cars/                # Vehicle information
│   ├── 04-Tracks/              # Track information
│   ├── 05-Stats/               # Member and career statistics
│   ├── 06-Results/             # Race results and subsessions
│   ├── 07-Series/              # Series information
│   ├── 08-Leagues/             # League data
│   ├── 09-Lookup/              # Design/avatar lookup
│   ├── 10-Constants/           # Static reference data
│   ├── 11-Documentation/       # API schema and documentation
│   ├── 12-DriverStats/         # Driver-specific statistics
│   ├── 13-Hosted/              # Hosted session data
│   ├── 14-Season/              # Season information
│   ├── 15-Session/             # Session and event data
│   ├── 16-Team/                # Team information
│   └── 17-TimeAttack/          # Time attack events
├── app.py                      # Web interface Flask app (coming soon)
├── oauth_client.py             # OAuth2 helper utilities (coming soon)
├── templates/                  # HTML templates for web UI (coming soon)
├── static/                     # CSS, JS for web UI (coming soon)
├── environments/               # Environment variable files
├── README.md                   # This file
├── LICENSE
└── .gitignore
```

## 🔗 How S3 Data Handling Works

The iRacing API uses S3 links to serve data files. APEX automates this process:

### Simple S3 Links (Most Endpoints)
1. API request returns `{link: "https://...", expires: "..."}`
2. Collection script automatically stores the link
3. Run "Fetch-S3-Data" to download the actual data

**Example Flow**:
```
Get Member Info → Get S3 link → Fetch-S3-Data → View member data
```

### Chunked Data (Search Endpoints)
Some search endpoints return data split across multiple files:

1. API request returns chunk info with multiple file URLs
2. Collection script stores chunk navigation info
3. "Fetch-S3-Data" retrieves the current chunk (default: chunk 1)
4. Adjust `s3_chunk_index` in Environment to switch chunks
5. Use "Chunk-Navigator" to see available chunks

**Example Flow**:
```
Search Results → Get chunk info → Fetch chunk 1 → Change index → Fetch chunk 2 → etc.
```

### Direct Data (Rare)
A few endpoints return data directly without S3 links:
- Response appears immediately in the body
- No additional fetch needed
- Collection displays a helpful preview

## 🛠️ Helper Requests

### Fetch-S3-Data
Universal S3 data fetcher that handles both simple links and chunked data:
- **Simple**: Fetches the stored S3 link
- **Chunked**: Fetches the current chunk (controlled by `s3_chunk_index`)
- Auto-detects response type and displays relevant information

**To fetch a different chunk**:
1. Go to Environment variables
2. Find `s3_chunk_index` (0-based)
3. Change to desired chunk number (0, 1, 2, ...)
4. Run "Fetch-S3-Data" again

### Chunk-Navigator
Visual browser for chunked data responses:
- Shows all available chunks
- Displays current position
- Provides easy chunk-switching instructions

## 🔄 Typical Workflows

### Workflow 1: Simple Data Fetch
```
1. Run any simple endpoint (Get Member Info, Get Car Info, etc.)
2. See S3 link in console output
3. Run "Fetch-S3-Data"
4. View data in response body
```

### Workflow 2: Search Results
```
1. Run search endpoint (Data Result Search Races, etc.)
2. See chunk info: "Chunk 1 of 5" (for example)
3. Run "Fetch-S3-Data" to get chunk 1
4. Set s3_chunk_index to 1, run again to get chunk 2
5. Repeat for each chunk needed
```

### Workflow 3: Using Chunk Navigator
```
1. After getting chunked data response
2. Run "Chunk-Navigator" to see all chunks
3. Note the chunk number you want
4. Update s3_chunk_index accordingly
5. Run "Fetch-S3-Data"
```

## ⚙️ Configuration

### Client ID
The collection is pre-configured with the APEX public client ID: `apex-api-explorer`

No additional setup is needed - the first request will prompt you to authenticate.

### Environment Variables
APEX automatically manages these after each request:
- `s3_link`: Current S3 link or first chunk URL
- `s3_type`: Response type (simple, chunked, or direct)
- `s3_expires`: Link expiration time
- `s3_chunk_count`: Total chunks (if chunked)
- `s3_chunk_index`: Current chunk (0-based, if chunked)
- `s3_base_url`: Base URL for chunks (if chunked)
- `s3_chunk_files`: Array of chunk filenames (if chunked)

You can view/edit these in Bruno's Environment panel.

## 🔗 Endpoint Categories

### Data Access (Member Info, Statistics, Results)
- **02-Member**: Profile, account info, login history
- **05-Stats**: Career stats, season stats, member stats
- **06-Results**: Race results, subsession details
- **12-DriverStats**: Driver-specific statistics
- **13-Hosted**: Hosted session results

### Reference Data (Cars, Tracks, Series)
- **03-Cars**: Vehicle specs and assets
- **04-Tracks**: Track details and layouts
- **07-Series**: Race series information
- **10-Constants**: Static reference data (tracks, cars, seasons)
- **14-Season**: Season schedule and details

### Social Features (Leagues, Teams)
- **08-Leagues**: League information and standings
- **16-Team**: Team data and standings

### Search & Utility
- **09-Lookup**: Helmets, paint patterns, sponsors, avatars
- **11-Documentation**: API documentation and schemas
- **15-Session**: Event sessions and schedules
- **17-TimeAttack**: Time attack event data

## 🐛 Troubleshooting

### "No S3 link found in environment"
- Make sure you ran an API request first (like "Get Member Info")
- Check that the request returned status 200
- The collection's post-response script should have logged the link

### S3 link expired (404 error)
- S3 links expire in ~2 minutes
- Run the original API request again to get a fresh link
- Then run "Fetch-S3-Data" immediately

### Chunk index out of range
- Check "Chunk-Navigator" to see how many chunks are available
- `s3_chunk_index` is 0-based (0, 1, 2, ...)
- For "Chunk 5 of 10", use index 4

### Authentication issues
- Check that you're using iRacing's official credentials
- Tokens refresh automatically - you shouldn't need to re-authenticate
- If stuck, try re-importing the collection or clearing Bruno's cache

## 📝 Response Types

APEX handles three types of API responses:

| Type | Pattern | How to Get Data | Example |
|------|---------|-----------------|---------|
| **Simple S3 Link** | `{link: "...", expires: "..."}` | Run "Fetch-S3-Data" once | Car info, track info, member profile |
| **Chunked Data** | `{data: {chunk_info: {...}}}` | Adjust `s3_chunk_index`, run multiple times | Search results, large datasets |
| **Direct Data** | Raw JSON (no S3 link) | Already in response body | Some lookup endpoints |

## 📖 Documentation

- iRacing API Docs: [members-ng.iracing.com/data/doc](https://members-ng.iracing.com/data/doc)
- Bruno Documentation: [www.usebruno.com](https://www.usebruno.com)
- OAuth 2.1 Specification: [oauth.net](https://oauth.net/)

## 📄 License

See LICENSE file for details.
