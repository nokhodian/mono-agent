// Static help guides for every platform+method that requires manual credentials.
// Each guide has: title, getKeyURL (primary action link), steps (array of strings or {text, url}).

export const HELP_GUIDES = {

  // ── Social ────────────────────────────────────────────────────────────────

  telegram: {
    apikey: {
      title: 'Create a Telegram Bot Token',
      getKeyURL: 'https://t.me/BotFather',
      steps: [
        'Open Telegram and search for @BotFather (the official bot creation bot)',
        'Send the command /newbot',
        'Choose a display name for your bot (any name)',
        'Choose a username ending in "bot" (e.g. myapp_bot)',
        'BotFather replies with your API token — copy it',
      ],
    },
  },

  // ── Services ──────────────────────────────────────────────────────────────

  github: {
    oauth: {
      title: 'Create a GitHub OAuth App',
      getKeyURL: 'https://github.com/settings/developers',
      steps: [
        { text: 'Go to GitHub → Settings → Developer settings → OAuth Apps', url: 'https://github.com/settings/developers' },
        'Click New OAuth App',
        'Fill in: Application name (e.g. "Monoes"), Homepage URL (any), Authorization callback URL: http://localhost:9876/callback',
        'Click Register application',
        'Copy the Client ID shown on the app page',
        'Click Generate a new client secret — copy it immediately (shown only once)',
        'Paste both values into the Client ID and Client Secret fields here',
      ],
    },
    apikey: {
      title: 'Generate a GitHub Personal Access Token',
      getKeyURL: 'https://github.com/settings/tokens/new',
      steps: [
        { text: 'Go to GitHub.com and click your profile photo → Settings', url: 'https://github.com/settings/profile' },
        'Scroll to Developer settings → Personal access tokens → Tokens (classic)',
        'Click Generate new token (classic)',
        'Add a note (e.g. "Monoes"), set expiration, and check scopes: repo, read:user, user:email',
        'Click Generate token at the bottom — copy it immediately, it won\'t be shown again',
      ],
    },
  },

  notion: {
    oauth: {
      title: 'Create a Notion OAuth Integration',
      getKeyURL: 'https://www.notion.so/my-integrations',
      steps: [
        { text: 'Go to notion.so/my-integrations', url: 'https://www.notion.so/my-integrations' },
        'Click + New integration',
        'Set Type to Public (enables OAuth)',
        'Add a Redirect URI: http://localhost:9876/callback',
        'Under Capabilities, enable Read content, Update content, Insert content',
        'Click Submit — on the next page go to the OAuth Domain & URIs section',
        'Copy the OAuth client ID and client secret shown there',
        'Paste both values into the Client ID and Client Secret fields here',
      ],
    },
    apikey: {
      title: 'Create a Notion Integration Token',
      getKeyURL: 'https://www.notion.so/my-integrations',
      steps: [
        { text: 'Go to notion.so/my-integrations', url: 'https://www.notion.so/my-integrations' },
        'Click + New integration',
        'Give it a name and select the workspace to connect',
        'Under Capabilities, enable Content, Comment, and User capabilities as needed',
        'Click Submit — copy the Internal Integration Token shown on the next page',
        'Important: open each Notion page/database you want to access and click Share → invite your integration',
      ],
    },
  },

  airtable: {
    oauth: {
      title: 'Register an Airtable OAuth Integration',
      getKeyURL: 'https://airtable.com/create/oauth',
      steps: [
        { text: 'Go to airtable.com/create/oauth', url: 'https://airtable.com/create/oauth' },
        'Click Register new integration',
        'Give it a name and set the OAuth redirect URL: http://localhost:9876/callback',
        'Select the scopes you need (data.records:read, data.records:write, schema.bases:read)',
        'Click Register integration',
        'Copy the Client ID and Client secret shown on the integration page',
        'Paste both values into the Client ID and Client Secret fields here',
      ],
    },
    apikey: {
      title: 'Create an Airtable Personal Access Token',
      getKeyURL: 'https://airtable.com/create/tokens',
      steps: [
        { text: 'Go to airtable.com/create/tokens', url: 'https://airtable.com/create/tokens' },
        'Click Create new token',
        'Name the token and add scopes: data.records:read, data.records:write, schema.bases:read',
        'Under Access, select the bases this token should access',
        'Click Create token — copy the token shown (it won\'t be shown again)',
      ],
    },
  },

  jira: {
    oauth: {
      title: 'Create a Jira (Atlassian) OAuth App',
      getKeyURL: 'https://developer.atlassian.com/console/myapps/',
      steps: [
        { text: 'Go to developer.atlassian.com/console/myapps/', url: 'https://developer.atlassian.com/console/myapps/' },
        'Click Create → OAuth 2.0 integration',
        'Give it a name and click Create',
        'Under Authorization, add a callback URL: http://localhost:9876/callback',
        'Under Permissions → Jira API, add scopes: read:me, read:jira-user, read:jira-work, write:jira-work',
        'Go to Settings and copy the Client ID and Secret',
        'Paste both values into the Client ID and Client Secret fields here',
      ],
    },
    apikey: {
      title: 'Get Your Jira API Token',
      getKeyURL: 'https://id.atlassian.com/manage-profile/security/api-tokens',
      steps: [
        { text: 'Go to id.atlassian.com → Security → API tokens', url: 'https://id.atlassian.com/manage-profile/security/api-tokens' },
        'Click Create API token, give it a label, and click Create',
        'Copy the token — it won\'t be shown again',
        'Your Email is the Atlassian account email you log in with',
        'Your Jira Domain is the part before .atlassian.net — e.g. if your Jira is mycompany.atlassian.net, enter mycompany',
      ],
    },
  },

  linear: {
    oauth: {
      title: 'Create a Linear OAuth Application',
      getKeyURL: 'https://linear.app/settings/api/applications/new',
      steps: [
        { text: 'Go to Linear → Settings → API → Create new application', url: 'https://linear.app/settings/api/applications/new' },
        'Fill in the app name and description',
        'Set the Callback URL to: http://localhost:9876/callback',
        'Click Create application',
        'Copy the Client ID and Client secret from the application page',
        'Paste both values into the Client ID and Client Secret fields here',
      ],
    },
    apikey: {
      title: 'Generate a Linear API Key',
      getKeyURL: 'https://linear.app/settings/api',
      steps: [
        { text: 'Open Linear and go to Settings → API', url: 'https://linear.app/settings/api' },
        'Scroll to Personal API keys and click Create key',
        'Give the key a label and click Create',
        'Copy the key — it won\'t be visible again',
      ],
    },
  },

  asana: {
    oauth: {
      title: 'Register an Asana OAuth App',
      getKeyURL: 'https://app.asana.com/0/my-apps',
      steps: [
        { text: 'Go to app.asana.com/0/my-apps', url: 'https://app.asana.com/0/my-apps' },
        'Click Create new app',
        'Give the app a name and click Create app',
        'Under OAuth, add a redirect URL: http://localhost:9876/callback',
        'Save the app',
        'Copy the Client ID and Client secret shown on the app page',
        'Paste both values into the Client ID and Client Secret fields here',
      ],
    },
    apikey: {
      title: 'Create an Asana Personal Access Token',
      getKeyURL: 'https://app.asana.com/0/my-apps',
      steps: [
        { text: 'Go to app.asana.com/0/my-apps', url: 'https://app.asana.com/0/my-apps' },
        'Click + Create new token',
        'Enter a name for the token and accept the terms',
        'Copy the token shown — it won\'t be shown again',
      ],
    },
  },

  stripe: {
    apikey: {
      title: 'Get Your Stripe Secret Key',
      getKeyURL: 'https://dashboard.stripe.com/apikeys',
      steps: [
        { text: 'Go to Stripe Dashboard → Developers → API keys', url: 'https://dashboard.stripe.com/apikeys' },
        'Use the Test mode key (sk_test_...) for development or Live key (sk_live_...) for production',
        'Click Reveal live key token or copy the visible test key',
        'Secret keys start with sk_test_ or sk_live_ — never share them',
      ],
    },
  },

  shopify: {
    apikey: {
      title: 'Get a Shopify Custom App Access Token',
      getKeyURL: 'https://help.shopify.com/en/manual/apps/app-types/custom-apps',
      steps: [
        'In your Shopify admin, go to Settings → Apps and sales channels',
        'Click Develop apps → Create an app',
        'Name the app and click Create app',
        'Click Configure Admin API scopes and select the permissions you need',
        'Click Save, then click Install app',
        'Copy the Admin API access token shown (only visible once)',
        'Your Shop Domain is yourstore.myshopify.com — enter just the subdomain part',
      ],
    },
  },

  hubspot: {
    oauth: {
      title: 'Create a HubSpot OAuth App',
      getKeyURL: 'https://developers.hubspot.com/get-started',
      steps: [
        { text: 'Go to developers.hubspot.com and log in with your HubSpot account', url: 'https://developers.hubspot.com/get-started' },
        'Click Create app (or open an existing developer account first)',
        'Give the app a name',
        'Under Auth → Redirect URLs, add: http://localhost:9876/callback',
        'Under Scopes, add the scopes you need (e.g. crm.objects.contacts.read)',
        'Click Save changes',
        'Copy the App ID (this is your Client ID) and Client secret from the Auth tab',
        'Paste both values into the Client ID and Client Secret fields here',
      ],
    },
    apikey: {
      title: 'Create a HubSpot Private App Token',
      getKeyURL: 'https://app.hubspot.com/private-apps',
      steps: [
        { text: 'In HubSpot, go to Settings → Integrations → Private Apps', url: 'https://app.hubspot.com/private-apps' },
        'Click Create a private app',
        'On the Basic Info tab, give the app a name',
        'On the Scopes tab, add the scopes you need (e.g. crm.objects.contacts.read)',
        'Click Create app → Continue creating',
        'Copy the access token — it won\'t be shown again after you leave the page',
      ],
    },
  },

  gmail: {
    oauth: {
      title: 'Set Up Gmail OAuth (Google Cloud)',
      getKeyURL: 'https://console.cloud.google.com/apis/credentials',
      steps: [
        { text: 'Go to console.cloud.google.com and create a new project (or select one)', url: 'https://console.cloud.google.com/projectcreate' },
        { text: 'Enable the Gmail API for your project', url: 'https://console.cloud.google.com/apis/library/gmail.googleapis.com' },
        { text: 'Go to APIs & Services → Credentials → Create Credentials → OAuth client ID', url: 'https://console.cloud.google.com/apis/credentials' },
        'Under Application type, choose Desktop app',
        'Give it a name and click Create',
        'Copy the Client ID and Client Secret shown in the dialog',
        'If prompted to configure the OAuth consent screen: set User Type to External, add your email as a test user, add scope: https://mail.google.com/',
        'Paste both values into the Client ID and Client Secret fields here',
      ],
    },
    apppassword: {
      title: 'Generate a Gmail App Password',
      getKeyURL: 'https://myaccount.google.com/apppasswords',
      steps: [
        'You must have 2-Step Verification enabled on your Google account first',
        { text: 'Go to myaccount.google.com/apppasswords', url: 'https://myaccount.google.com/apppasswords' },
        'Under "Select app" choose Mail; under "Select device" choose your device type',
        'Click Generate — Google shows a 16-character app password',
        'Copy the password (spaces are optional — you can include or omit them)',
        'Use this app password in the Password field, along with your full Gmail address',
      ],
    },
  },

  slack: {
    oauth: {
      title: 'Create a Slack OAuth App',
      getKeyURL: 'https://api.slack.com/apps/new',
      steps: [
        { text: 'Go to api.slack.com/apps and click Create New App → From scratch', url: 'https://api.slack.com/apps/new' },
        'Give the app a name and select the workspace',
        'Under OAuth & Permissions → Redirect URLs, add: http://localhost:9876/callback',
        'Under Scopes → Bot Token Scopes, add scopes you need (e.g. channels:read, chat:write, users:read)',
        'Click Install to Workspace (or use OAuth for user tokens)',
        'Go to Basic Information and copy the Client ID and Client Secret',
        'Paste both values into the Client ID and Client Secret fields here',
      ],
    },
  },

  google_sheets: {
    oauth: {
      title: 'Set Up Google Sheets OAuth',
      getKeyURL: 'https://console.cloud.google.com/apis/credentials',
      steps: [
        { text: 'Go to console.cloud.google.com and create a new project (or select one)', url: 'https://console.cloud.google.com/projectcreate' },
        { text: 'Enable the Google Sheets API for your project', url: 'https://console.cloud.google.com/apis/library/sheets.googleapis.com' },
        { text: 'Go to APIs & Services → Credentials → Create Credentials → OAuth client ID', url: 'https://console.cloud.google.com/apis/credentials' },
        'Under Application type, choose Desktop app',
        'Give it a name and click Create',
        'Copy the Client ID and Client Secret shown in the dialog',
        'If prompted to configure the OAuth consent screen: set User Type to External, add your email as a test user, add scope: https://www.googleapis.com/auth/spreadsheets',
        'Paste both values into the Client ID and Client Secret fields here',
      ],
    },
  },

  google_drive: {
    oauth: {
      title: 'Set Up Google Drive OAuth',
      getKeyURL: 'https://console.cloud.google.com/apis/credentials',
      steps: [
        { text: 'Go to console.cloud.google.com and create a new project (or select one)', url: 'https://console.cloud.google.com/projectcreate' },
        { text: 'Enable the Google Drive API for your project', url: 'https://console.cloud.google.com/apis/library/drive.googleapis.com' },
        { text: 'Go to APIs & Services → Credentials → Create Credentials → OAuth client ID', url: 'https://console.cloud.google.com/apis/credentials' },
        'Under Application type, choose Desktop app',
        'Give it a name and click Create',
        'Copy the Client ID and Client Secret shown in the dialog',
        'If prompted to configure the OAuth consent screen: set User Type to External, add your email as a test user, add scope: https://www.googleapis.com/auth/drive',
        'Paste both values into the Client ID and Client Secret fields here',
      ],
    },
  },

  salesforce: {
    oauth: {
      title: 'Create a Salesforce Connected App',
      getKeyURL: 'https://login.salesforce.com',
      steps: [
        'Log in to Salesforce and go to Setup (gear icon → Setup)',
        'In the Quick Find box, search for "App Manager" and click it',
        'Click New Connected App',
        'Fill in: Connected App Name, API Name, Contact Email',
        'Check Enable OAuth Settings',
        'Set Callback URL to: http://localhost:9876/callback',
        'Add OAuth Scopes: Access and manage your data (api), Perform requests at any time (refresh_token)',
        'Click Save, then Continue',
        'Copy the Consumer Key (this is your Client ID) and Consumer Secret (Client Secret)',
        'Paste both values into the Client ID and Client Secret fields here',
      ],
    },
  },

  // ── Communication ─────────────────────────────────────────────────────────

  discord: {
    apikey: {
      title: 'Create a Discord Bot Token',
      getKeyURL: 'https://discord.com/developers/applications',
      steps: [
        { text: 'Go to discord.com/developers/applications', url: 'https://discord.com/developers/applications' },
        'Click New Application, give it a name, and click Create',
        'In the left sidebar click Bot, then Add Bot → Yes, do it!',
        'Under Token, click Reset Token and confirm — copy the token',
        'Enable any Privileged Gateway Intents you need (Message Content, Server Members, etc.)',
        'To add the bot to a server: OAuth2 → URL Generator → select bot scope → copy and open the URL',
      ],
    },
  },

  twilio: {
    apikey: {
      title: 'Get Your Twilio Credentials',
      getKeyURL: 'https://console.twilio.com/',
      steps: [
        { text: 'Log in to console.twilio.com', url: 'https://console.twilio.com/' },
        'Your Account SID and Auth Token are on the Dashboard home page',
        'Click the eye icon next to Auth Token to reveal it',
        'For the From Number: go to Phone Numbers → Manage → Active Numbers and copy a number',
        'If you have no number, click Buy a number and purchase one (or use the free trial number)',
        'Use the full E.164 format: +1XXXXXXXXXX',
      ],
    },
  },

  whatsapp: {
    apikey: {
      title: 'Get WhatsApp Business API Credentials',
      getKeyURL: 'https://developers.facebook.com/',
      steps: [
        { text: 'Go to developers.facebook.com and create or open a Meta app', url: 'https://developers.facebook.com/' },
        'Add the WhatsApp product to your app',
        'Go to WhatsApp → API Setup',
        'Your Account SID is shown as "Phone number ID", Auth Token as the temporary access token',
        'For a permanent token: create a System User in Meta Business Manager with admin access',
        'Your From Number is the test number shown on the API Setup page (or a verified business number)',
      ],
    },
  },

  smtp: {
    apppassword: {
      title: 'Configure SMTP / Email',
      getKeyURL: 'https://support.google.com/mail/answer/7126229',
      steps: [
        'Enter your full email address and password (or app password for Gmail/Yahoo)',
        'For Gmail SMTP: smtp.gmail.com port 587 (TLS) or 465 (SSL)',
        'For Outlook/Hotmail: smtp.office365.com port 587',
        'For Yahoo: smtp.mail.yahoo.com port 587',
        'For IMAP (receiving): Gmail uses imap.gmail.com port 993',
        'Gmail users: use an App Password instead of your regular password (requires 2FA)',
      ],
    },
  },

  // ── Databases ─────────────────────────────────────────────────────────────

  postgresql: {
    connstring: {
      title: 'PostgreSQL Connection String',
      getKeyURL: 'https://www.postgresql.org/docs/current/libpq-connect.html#LIBPQ-CONNSTRING',
      steps: [
        'Format: postgresql://username:password@host:port/database',
        'Example: postgresql://alice:secret@localhost:5432/myapp',
        'For SSL: add ?sslmode=require or ?sslmode=disable at the end',
        'Cloud providers (Supabase, Railway, Render, Neon): copy the connection string from your dashboard',
        'For Supabase: Settings → Database → Connection string → URI',
        'For Railway: click your PostgreSQL service → Connect tab',
      ],
    },
  },

  mysql: {
    connstring: {
      title: 'MySQL Connection String',
      getKeyURL: 'https://pkg.go.dev/github.com/go-sql-driver/mysql#readme-dsn-data-source-name',
      steps: [
        'Format: username:password@tcp(host:port)/database',
        'Example: alice:secret@tcp(localhost:3306)/myapp',
        'For SSL: append ?tls=true or ?tls=skip-verify',
        'For PlanetScale / AWS RDS: copy the connection string from your dashboard',
        'Cloud providers often require SSL — use ?tls=true for hosted MySQL',
      ],
    },
  },

  mongodb: {
    connstring: {
      title: 'MongoDB Connection String',
      getKeyURL: 'https://www.mongodb.com/docs/manual/reference/connection-string/',
      steps: [
        'Format: mongodb://username:password@host:port/database',
        'For MongoDB Atlas: mongodb+srv://username:password@cluster0.xxxxx.mongodb.net/mydb',
        'Atlas: click Connect on your cluster → Drivers → copy the connection string',
        'Replace <password> with your actual password in the Atlas string',
        'Add ?retryWrites=true&w=majority for Atlas (usually pre-filled)',
      ],
    },
  },

  redis: {
    connstring: {
      title: 'Redis Connection String',
      getKeyURL: 'https://redis.io/docs/manual/cli/',
      steps: [
        'Format: redis://:password@host:port/db-number',
        'No password: redis://localhost:6379/0',
        'With password: redis://:mysecret@localhost:6379/0',
        'For Upstash: copy the Redis URL from your database dashboard',
        'For Railway: click your Redis service → Connect tab → copy the URL',
        'For Redis Cloud: go to your database → Configuration → copy the endpoint + password',
      ],
    },
  },
}
