# eng-reminder

GitHub Actions bot yang secara otomatis memantau tiket **Bug** di Jira setiap **5 menit** dan mengirim notifikasi ke **Discord**. Bot ini mencakup empat jenis notifikasi:

| # | Notifikasi | Trigger |
|---|-----------|---------|
| 1 | **New Bug Alert** | Ada tiket Bug baru dengan status `To Do` |
| 2 | **Hanging Bug (To Do)** | Bug stuck di `To Do` > threshold menit |
| 3 | **Hanging Code Review** | Bug stuck di `Code Review` > threshold menit |
| 4 | **Open Bug Reminder** | Ringkasan semua bug yang belum selesai |

Severity hanging alert dihitung dari jumlah tiket:

| Jumlah Tiket | Severity |
|:---:|:---:|
| < 10 | 🟡 LOW |
| 10 – 14 | 🟠 MIDDLE |
| ≥ 15 | 🔴 HIGH |

---

## Struktur Folder

```
eng-reminder/
├── .github/
│   └── workflows/
│       └── jira-bug-reminder.yml   # GitHub Actions — cron */5 * * * *
├── cmd/
│   └── main.go                     # Entry point
├── internal/
│   ├── config/
│   │   └── config.go               # Baca env vars & validasi
│   ├── jira/
│   │   └── client.go               # Jira REST API v3 client
│   └── notifier/
│       └── discord.go              # Discord Incoming Webhook notifier
├── .env                            # Env vars untuk local dev (jangan di-commit)
├── .gitignore
├── go.mod
├── go.sum
└── README.md
```

---

## Instalasi & Setup

### Prasyarat

- **Go 1.21+** — [download](https://go.dev/dl/)
- Akses ke Jira (Atlassian Cloud)
- Discord server dengan permission **Manage Webhooks**

---

### 1. Clone Repository

```bash
git clone https://github.com/lionparcel/eng-reminder.git
cd eng-reminder
```

### 2. Install Dependencies

```bash
go mod download
```

### 3. Buat Jira API Token

1. Buka [https://id.atlassian.com/manage-profile/security/api-tokens](https://id.atlassian.com/manage-profile/security/api-tokens)
2. Klik **Create API token**
3. Beri nama (contoh: `eng-reminder`) → **Create**
4. Salin token yang muncul

### 4. Buat Discord Webhook

1. Buka Discord server → pilih channel tujuan
2. Klik ⚙️ **Edit Channel** → **Integrations** → **Webhooks**
3. Klik **New Webhook** → beri nama (contoh: `eng-reminder`)
4. Klik **Copy Webhook URL**
5. Simpan URL tersebut untuk diisi di `.env`

### 5. Konfigurasi Env Vars

Salin file `.env` dan isi nilainya:

```bash
cp .env .env.local   # opsional, atau langsung edit .env
```

```env
# Jira
JIRA_BASE_URL=https://yourcompany.atlassian.net
JIRA_EMAIL=your-email@company.com
JIRA_API_TOKEN=your_jira_api_token
JIRA_MAX_RESULTS=10

# Menit lookback untuk new bug alert (default: 15)
JIRA_NEW_BUG_WINDOW_MINUTES=15

# Menit threshold bug stuck di To Do (default: 10)
BUG_HANGING_MINUTES=10

# Menit threshold bug stuck di Code Review (default: 10)
CODE_REVIEW_HANGING_MINUTES=10

# Discord
DISCORD_WEBHOOK_URL=https://discord.com/api/webhooks/xxx/yyy

# Discord user ID lead engineer yang akan di-mention (numeric snowflake)
# Cara dapat: Discord → Settings → Advanced → Enable Developer Mode
#             Lalu klik kanan user → Copy User ID
DISCORD_LEAD_IDS=123456789012345678,987654321098765432
```

### 6. Jalankan Lokal

```bash
# Load env dan jalankan
export $(grep -v '^#' .env | xargs)
go run ./cmd/main.go
```

Atau build dulu:

```bash
go build -o eng-reminder ./cmd/main.go
./eng-reminder
```

---

## Setup GitHub Actions (Production)

### Secrets (wajib)

Tambahkan di **Settings → Secrets and variables → Actions → Secrets**:

| Secret | Nilai |
|--------|-------|
| `JIRA_BASE_URL` | `https://yourcompany.atlassian.net` |
| `JIRA_EMAIL` | Email akun Jira |
| `JIRA_API_TOKEN` | API Token dari Atlassian |
| `DISCORD_WEBHOOK_URL` | Discord Incoming Webhook URL |

### Variables (opsional, ada default)

Tambahkan di **Settings → Secrets and variables → Actions → Variables**:

| Variable | Default | Keterangan |
|----------|---------|------------|
| `JIRA_MAX_RESULTS` | `10` | Jumlah maks bug di open bug reminder |
| `JIRA_NEW_BUG_WINDOW_MINUTES` | `15` | Lookback window new bug alert (menit) |
| `BUG_HANGING_MINUTES` | `10` | Threshold bug hanging di To Do (menit) |
| `CODE_REVIEW_HANGING_MINUTES` | `10` | Threshold bug hanging di Code Review (menit) |
| `DISCORD_LEAD_IDS` | _(kosong)_ | Discord user IDs lead engineer, pisah koma |

### Trigger Manual

Workflow bisa dijalankan manual tanpa menunggu cron:

1. Buka tab **Actions** di GitHub
2. Pilih workflow **🐛 Jira Bug Reminder**
3. Klik **Run workflow**

---

## Contoh Notifikasi Discord

**New Bug Alert**
> 🆕 **[New Bug Alert] Ada bug baru — mohon segera ditindaklanjuti!**
> 1 bug baru dengan status **To Do** ditemukan.
>
> 🔴 **[LP-1250] Checkout gagal di iOS 17**
> Status: To Do | Priority: Critical
> Assignee: _Unassigned_ | Reporter: Siti Rahma
> Dibuat: 06 May 2026 10:00 (2m yang lalu)
> [🔗 Buka di Jira](https://yourcompany.atlassian.net/browse/LP-1250)

---

**Hanging Bug Alert (To Do) — MIDDLE**
> ⚙️ **ORANGE ALERT — Bug Menunggu Fixing Engineer**
> Bug dalam fase development telah mencapai batas **MIDDLE** 🟠
>
> 📌 **Epic**
> [PROJ-100] Nama Epic
> https://yourcompany.atlassian.net/browse/PROJ-100
>
> 🦎 **Jumlah Bug (Dev Phase)** &nbsp;&nbsp;&nbsp;&nbsp; 📊 **Threshold**
> **10** bug &nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp; 🔴 High: 15 | 🟠 Mid: 10 | 🟡 Low: 6
>
> 👤 **Breakdown per Assignee**
> **Andi**: 4 Bug | **Budi**: 3 Bug | **Citra**: 2 Bug | **Dian**: 1 Bug
>
> 🎯 **Triggered By**
> [LP-1245] Push notification tidak terkirim
> https://yourcompany.atlassian.net/browse/LP-1245
>
> 📋 **Status yang Dihitung**
> `Todo` | `In Progress` | `Code Review` | `Rejected` | `Reject`
>
> _Jira Bug Monitor • Development Phase Alert_

---

**Hanging Code Review Alert — HIGH**
> ⚙️ **RED ALERT — Bug Menunggu Code Review Lead**
> Bug dalam fase code review telah mencapai batas **HIGH** 🔴
>
> 📌 **Epic**
> [PROJ-100] Nama Epic
> https://yourcompany.atlassian.net/browse/PROJ-100
>
> 🦎 **Jumlah Bug (Code Review Phase)** &nbsp;&nbsp;&nbsp;&nbsp; 📊 **Threshold**
> **15** bug &nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp; 🔴 High: 15 | 🟠 Mid: 10 | 🟡 Low: 6
>
> 👤 **Breakdown per Assignee**
> **Budi**: 5 Bug | **Andi**: 4 Bug | **Citra**: 3 Bug | **Dian**: 2 Bug | **Eko**: 1 Bug
>
> 🎯 **Triggered By**
> [LP-1230] Payment timeout on checkout
> https://yourcompany.atlassian.net/browse/LP-1230
>
> 📋 **Status yang Dihitung**
> `Code Review`
>
> _Jira Bug Monitor • Code Review Phase Alert_

---

## Format Mention Discord

| Nilai `DISCORD_LEAD_IDS` | Hasil |
|--------------------------|-------|
| `123456789012345678` | `<@123456789012345678>` — mention user spesifik |
| `123456789012345678,987654321098765432` | `<@123...> <@987...>` — multiple user |
| `here` | `@here` — mention semua member online |
| `everyone` | `@everyone` — mention semua member channel |


