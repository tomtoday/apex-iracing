# APEX - API Explorer for iRacing

> **The optimal line to your iRacing data**

## About OAuth Authentication

APEX is a Bruno collection that uses the OAuth 2.1 Authorization Code Flow with PKCE (Proof Key for Code Exchange) to securely access the iRacing Data API. This method is designed for public/shared applications where multiple users can use the same collection without sharing credentials.

### Public Client Architecture

APEX uses a **public client** model, which means:

- **One shared `client_id`**: All users of APEX use the same client identifier, which is included in this repository
- **No `client_secret`**: Public clients don't receive a client secret, eliminating the risk of secret leakage in shared code
- **PKCE for security**: Each authentication flow generates unique, cryptographically random codes that prevent authorization code interception attacks
- **User-specific tokens**: Every user authenticates individually through iRacing's official login page and receives their own access/refresh tokens
- **Local-only storage**: Tokens are stored in each user's local Bruno environment and never committed to the repository

This architecture is the recommended OAuth 2.1 approach for CLI tools, desktop applications, and shared API collections. It ensures that:
- Users always authenticate directly with iRacing (not through a third party)
- Credentials are never exposed to the collection maintainer or other users
- The collection can be safely shared on GitHub without security concerns
- Each user maintains full control over their own data access

### Registration Details

This collection is registered with iRacing as:
- **Client Name**: APEX - API Explorer for iRacing
- **Client Type**: Public client (single-page-app/native)
- **Audience**: data-server
- **Redirect URI**: `http://localhost:3000/callback`

The public `client_id` for this collection is: `[TBD_Need_client_id_from_iracing]`