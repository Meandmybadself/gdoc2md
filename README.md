# gdoc2md

A fast CLI tool that exports all tabs from a Google Doc as Markdown files.

## Features

- Exports every tab in a Google Doc to its own `.md` file
- Generates a `tabs.md` table of contents linking all exported documents
- Downloads inline images to a local `images/` directory
- Processes tabs and image downloads in parallel for speed
- Single binary with no runtime dependencies — builds for macOS, Linux, and Windows
- OAuth2 authentication with automatic token refresh

## Installation

### From source

Requires Go 1.24+.

```bash
go install github.com/meandmybadself/gdoc2md@latest
```

### From release binaries

Download the appropriate binary for your platform from the [Releases](https://github.com/meandmybadself/gdoc2md/releases) page.

### Build from source

```bash
git clone https://github.com/meandmybadself/gdoc2md.git
cd gdoc2md
make build        # Build for your current platform
make all          # Build for all platforms (output in dist/)
```

## Google Cloud Setup

Before using `gdoc2md`, you need to create a Google Cloud project with OAuth credentials. This is a one-time setup.

### 1. Create a Google Cloud Project

1. Go to the [Google Cloud Console](https://console.cloud.google.com)
2. Click the **project selector** dropdown at the top of the page
3. Click **New Project**
4. Enter a project name (e.g., "gdoc2md") and click **Create**
5. Make sure your new project is selected in the project selector

### 2. Enable the Google Docs API

1. In your project, navigate to **APIs & Services > Library** ([direct link](https://console.cloud.google.com/apis/library))
2. Search for **Google Docs API**
3. Click on it and then click **Enable**

### 3. Configure the OAuth Consent Screen

1. Navigate to **APIs & Services > OAuth consent screen** ([direct link](https://console.cloud.google.com/apis/credentials/consent))
2. Select **External** as the user type (unless you have a Google Workspace org and want Internal) and click **Create**
3. Fill in the required fields:
   - **App name**: `gdoc2md`
   - **User support email**: your email address
   - **Developer contact information**: your email address
4. Click **Save and Continue**
5. On the **Scopes** page, click **Add or Remove Scopes**
   - Search for `Google Docs API` and check `.../auth/documents.readonly`
   - Click **Update**, then **Save and Continue**
6. On the **Test users** page, click **Add Users**
   - Add your own Google email address (this is required while the app is in "Testing" status)
   - Click **Save and Continue**
7. Review and click **Back to Dashboard**

> **Note:** While your app is in "Testing" status, only the test users you add can authorize it. This is fine for personal use. Publishing the app removes this restriction but requires Google verification.

### 4. Create OAuth Credentials

1. Navigate to **APIs & Services > Credentials** ([direct link](https://console.cloud.google.com/apis/credentials))
2. Click **Create Credentials > OAuth client ID**
3. For **Application type**, select **Desktop app**
4. Enter a name (e.g., "gdoc2md CLI") and click **Create**
5. You will see your **Client ID** and **Client Secret** — save these, you'll need them next

## Usage

### Configure credentials (one-time)

```bash
gdoc2md configure
```

You will be prompted to enter the Client ID and Client Secret from step 4 above. These are stored in `~/.gdoc2md/config.json` with restricted file permissions.

### Export a Google Doc

```bash
# Export all tabs to the current directory
gdoc2md https://docs.google.com/document/d/YOUR_DOC_ID/edit

# Export to a specific directory
gdoc2md -o ./output https://docs.google.com/document/d/YOUR_DOC_ID/edit
```

On first run, your browser will open for Google authorization. After granting access, the token is cached in `~/.gdoc2md/token.json` and subsequent runs are automatic.

### Output structure

```
output/
├── tabs.md              # Table of contents
├── Tab Name One.md      # Markdown for each tab
├── Tab Name Two.md
└── images/
    ├── tab0_image_001.jpg
    └── tab1_image_001.png
```

### Flags

```
-o string    Output directory (default: current directory)
-version     Print version and exit
```

## How It Works

1. Fetches the Google Doc with all tab content in a single API call
2. Flattens the tab tree (including nested/child tabs)
3. Converts each tab to Markdown in parallel using goroutines
4. Downloads all referenced images in parallel (up to 10 concurrent)
5. Writes Markdown files and a `tabs.md` index

## Credential Storage

All credentials are stored in `~/.gdoc2md/` with restricted permissions:

| File | Contents | Permissions |
|------|----------|-------------|
| `config.json` | OAuth Client ID and Secret | `0600` (owner read/write only) |
| `token.json` | OAuth access and refresh tokens | `0600` (owner read/write only) |

To re-authenticate, delete `~/.gdoc2md/token.json` and run an export again.
To change OAuth credentials, run `gdoc2md configure` again.

## License

MIT
