"""OAuth2 client for iRacing authentication"""
import json
import os
import secrets
import webbrowser
from urllib.parse import urlencode, parse_qs
from urllib.request import urlopen
import requests

# iRacing OAuth configuration
CLIENT_ID = "apex-api-explorer"
CALLBACK_URL = "http://127.0.0.1:3000/callback"
AUTH_URL = "https://oauth.iracing.com/oauth2/authorize"
TOKEN_URL = "https://oauth.iracing.com/oauth2/token"
TOKEN_FILE = ".iracing_token"


class IracingOAuthClient:
    """Handles OAuth2 flow with iRacing"""

    def __init__(self):
        self.client_id = CLIENT_ID
        self.callback_url = CALLBACK_URL
        self.token = self._load_token()
        self.state = None
        self.code_verifier = None

    def _load_token(self):
        """Load token from file if it exists"""
        if os.path.exists(TOKEN_FILE):
            try:
                with open(TOKEN_FILE, 'r') as f:
                    return json.load(f)
            except (json.JSONDecodeError, IOError):
                return None
        return None

    def _save_token(self, token):
        """Save token to file"""
        with open(TOKEN_FILE, 'w') as f:
            json.dump(token, f, indent=2)
        self.token = token

    def clear_token(self):
        """Clear the token (delete file and reset)"""
        if os.path.exists(TOKEN_FILE):
            os.remove(TOKEN_FILE)
        self.token = None

    def is_authenticated(self):
        """Check if user is authenticated"""
        return self.token is not None and 'access_token' in self.token

    def get_access_token(self):
        """Get current access token, refreshing if needed"""
        if not self.token:
            return None

        # Check if token needs refresh (simple heuristic: if no expiry info, assume valid)
        # In production, you'd check token expiry time
        return self.token.get('access_token')

    def refresh_access_token(self):
        """Refresh expired access token using refresh_token"""
        if not self.token or 'refresh_token' not in self.token:
            return False

        try:
            data = {
                'grant_type': 'refresh_token',
                'client_id': self.client_id,
                'refresh_token': self.token['refresh_token'],
            }

            response = requests.post(TOKEN_URL, data=data)
            response.raise_for_status()

            new_token = response.json()
            self._save_token(new_token)
            return True
        except Exception as e:
            print(f"Token refresh failed: {str(e)}")
            return False

    def generate_auth_url(self):
        """Generate OAuth authorization URL with PKCE"""
        # Generate PKCE code challenge
        self.code_verifier = self._generate_pkce_verifier()
        code_challenge = self._generate_pkce_challenge(self.code_verifier)

        # Generate state for CSRF protection
        self.state = secrets.token_urlsafe(32)

        params = {
            'client_id': self.client_id,
            'response_type': 'code',
            'redirect_uri': self.callback_url,
            'scope': 'iracing.auth',
            'state': self.state,
            'code_challenge': code_challenge,
            'code_challenge_method': 'S256',
        }

        return f"{AUTH_URL}?{urlencode(params)}"

    def handle_callback(self, code, state):
        """Handle OAuth callback and exchange code for token"""
        # Verify state
        if state != self.state:
            raise ValueError("State mismatch - potential CSRF attack")

        # Exchange code for token using PKCE
        data = {
            'grant_type': 'authorization_code',
            'client_id': self.client_id,
            'code': code,
            'redirect_uri': self.callback_url,
            'code_verifier': self.code_verifier,
        }

        response = requests.post(TOKEN_URL, data=data)
        response.raise_for_status()

        token = response.json()
        self._save_token(token)
        return token

    @staticmethod
    def _generate_pkce_verifier():
        """Generate PKCE code verifier"""
        return secrets.token_urlsafe(32)

    @staticmethod
    def _generate_pkce_challenge(verifier):
        """Generate PKCE code challenge from verifier"""
        import hashlib
        import base64
        digest = hashlib.sha256(verifier.encode()).digest()
        return base64.urlsafe_b64encode(digest).decode().rstrip('=')

    def open_browser(self, auth_url):
        """Open browser for user to authenticate"""
        webbrowser.open(auth_url)
