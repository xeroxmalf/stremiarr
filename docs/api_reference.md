# Handoff API Reference

## Standard Endpoints

- **GET /health**
  - **Description**: Health check endpoint.
  - **Auth**: None
  - **Response**: Status 200 OK.

- **GET /** or **GET /dashboard**
  - **Description**: Web UI interface.
  - **Auth**: May require authentication depending on setup.
  - **Response**: HTML interface.

- **GET /metrics**
  - **Description**: Prometheus metrics endpoint.
  - **Auth**: None
  - **Response**: Prometheus formatted metrics.

- **GET /api/stats**
  - **Description**: Stream statistics.
  - **Auth**: Admin
  - **Response**: JSON with stream statistics.

- **GET /api/bandwidth**
  - **Description**: Bandwidth statistics.
  - **Auth**: Admin
  - **Response**: JSON with bandwidth usage.

- **GET /api/ws**
  - **Description**: WebSocket connection for real-time updates.
  - **Auth**: Requires valid session/token.
  - **Response**: Upgrades to WebSocket connection.

- **GET /api/keys**
  - **Description**: List Real-Debrid API keys.
  - **Auth**: Admin
  - **Response**: JSON array of keys.

- **POST /api/keys**
  - **Description**: Add Real-Debrid API key.
  - **Auth**: Admin
  - **Request Body**: JSON with key details.
  - **Response**: JSON confirmation.

- **DELETE /api/keys?token=...**
  - **Description**: Remove Real-Debrid API key.
  - **Auth**: Admin
  - **Response**: Status 200 OK.

- **GET /api/sources**
  - **Description**: List addon sources.
  - **Auth**: Admin
  - **Response**: JSON array of sources.

- **PUT /api/sources**
  - **Description**: Update addon sources.
  - **Auth**: Admin
  - **Request Body**: JSON with updated sources.
  - **Response**: JSON confirmation.

- **POST /api/mappings**
  - **Description**: Create alias mapping.
  - **Auth**: Admin
  - **Request Body**: JSON with mapping details.
  - **Response**: JSON confirmation.

- **DELETE /api/mappings?alias=...**
  - **Description**: Delete alias mapping.
  - **Auth**: Admin
  - **Response**: Status 200 OK.

- **POST /api/clean**
  - **Description**: Clean stale or invalid streams.
  - **Auth**: Admin
  - **Response**: JSON status.

- **GET /api/cache**
  - **Description**: Retrieve cache information.
  - **Auth**: Admin (or auth token)
  - **Response**: JSON cache stats.

- **GET /transcode?stream=...**
  - **Description**: Audio transcode endpoint.
  - **Auth**: None (or stream specific token)
  - **Response**: Transcoded audio stream.

- **POST /api/addons/discover**
  - **Description**: Discover and add a new addon.
  - **Auth**: Admin
  - **Request Body**: JSON with addon URL/details.
  - **Response**: JSON status.

- **DELETE /api/addons/discover?url=...**
  - **Description**: Remove discovered addon.
  - **Auth**: Admin
  - **Response**: Status 200 OK.

- **POST /api/prefetch**
  - **Description**: Start library prefetch.
  - **Auth**: Admin
  - **Request Body**: Optional JSON parameters.
  - **Response**: JSON status.

- **POST /api/prefetch/stop**
  - **Description**: Stop running prefetch.
  - **Auth**: Admin
  - **Response**: JSON status.

- **POST /api/stremio/auth**
  - **Description**: Set Stremio auth key.
  - **Auth**: User/Admin
  - **Request Body**: JSON with auth key.
  - **Response**: Status 200 OK.

- **GET /auth/login**
  - **Description**: OAuth2 login redirect.
  - **Auth**: None
  - **Response**: Redirect to OAuth provider.

- **GET /auth/callback**
  - **Description**: OAuth2 callback.
  - **Auth**: None
  - **Response**: Redirect with session/token.

## Stremio Addon Proxy Endpoints

- **GET /{alias}/manifest.json**
  - **Description**: Proxied addon manifest.
  - **Auth**: None
  - **Response**: JSON manifest.

- **GET /{alias}/catalog/{type}/{id}.json**
  - **Description**: Proxied catalog endpoint.
  - **Auth**: None
  - **Response**: JSON catalog response.

- **GET /{alias}/stream/{type}/{id}.json**
  - **Description**: Proxied streams (wraps Real-Debrid links).
  - **Auth**: None
  - **Response**: JSON stream response.

- **GET /{alias}/play?link=...**
  - **Description**: Play/proxy endpoint for media.
  - **Auth**: None
  - **Response**: Redirects to or streams media content.
