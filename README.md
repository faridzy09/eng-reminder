# eng-reminder

GitHub Actions bot yang memantau tiket **Bug** di Jira dan **kapasitas SP harian** engineer, lalu mengirim notifikasi ke **Discord**. Berjalan otomatis setiap jam kerja WIB (08:00–18:00).

---

## Jenis Notifikasi

| # | Notifikasi | Channel | Interval | Trigger |
|---|-----------|---------|---------|---------|
| 1 | **Hanging Bug (To Do)** | Bug channel | 15 menit | Bug stuck di `To Do` / `Rejected` sejak tanggal tertentu |
| 2 | **Hanging Code Review** | Bug channel | 15 menit | Bug stuck di `Code Review` sejak tanggal tertentu |
| 3 | **SP Capacity Check** | SP channel | 30 menit | Ringkasan SP harian per engineer, dikelompokkan per SPV || 4 | **Code Review Task Alert** | Code Review channel | 60 menit | Task engineer (Sub-task/Task) yang sedang di `Code Review`, dikelompokkan per SPV |
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
│       └── jira-bug-reminder.yml     # GitHub Actions workflow
├── cmd/
│   └── main.go                       # Entry point — dua ticker (15m bug, 30m SP)
├── internal/
│   ├── config/
│   │   └── config.go                 # Baca env vars & validasi
│   ├── engineer/
│   │   └── engineer.go               # Daftar 25 data engineer + SPV + SP/hari
│   ├── jira/
│   │   └── client.go                 # Jira REST API v3 client (POST /search/jql)
│   └── notifier/
│       └── discord.go                # Discord Incoming Webhook notifier
├── .env                              # Env vars untuk local dev (jangan di-commit)
├── .gitignore
├── go.mod
├── go.sum
└── README.md
```

---

## Tim Data Engineer

| No | Nama | SP/Hari | SPV |
|----|------|:-------:|-----|
| 1 | Yandra Charlos Hasugian | 8 | DeriKurniawan |
| 2 | Bayu Kurniawan | 6 | Falih Mulyana |
| 3 | Fiqri Ramadhan | 6 | Falih Mulyana |
| 4 | Muhamad Lutfi Alfiansyah | 6 | Falih Mulyana |
| 5 | Risyadul Alim | 6 | Falih Mulyana |
| 6 | Ridho Tanjung | 6 | Falih Mulyana |
| 7 | Adi Saputra | 6 | Faridho |
| 8 | Rizki Gumilar | 6 | Faridho |
| 9 | Andika Prasetya | 6 | Faridho |
| 10 | Naufal Hadi | 6 | Faridho |
| 11 | Fuad Rifqi Zamzami | 6 | Faridho |
| 12 | Andikha Apriadi | 6 | Faridho |
| 13 | Junifer Rionaldi Manik | 6 | Irvan Resna Hadiyana |
| 14 | M. Arif Sefrianto | 8 | Irvan Resna Hadiyana |
| 15 | Fadli Muhamad Paridi | 8 | Irvan Resna Hadiyana |
| 16 | Anom Yulian Hartanto | 6 | Muhammad Farid H |
| 17 | Yusuf Gutara | 6 | Muhammad Farid H |
| 18 | Fajrul Aulia | 6 | Sholahuddin Alisyahbana |
| 19 | Dani Mulyana | 6 | Susi Cahyati |
| 20 | Rosyid Rosadi | 6 | Susi Cahyati |
| 21 | Fajar Darwis | 6 | Susi Cahyati |
| 22 | Rifat Firdaus | 6 | Susi Cahyati |
| 23 | Clara Anggraini | 6 | Susi Cahyati |
| 24 | Indra Ikwal | 6 | DeriKurniawan |
| 25 | Pratama Egho | 6 | Faridho |

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

Buat **3 webhook** di channel yang berbeda:
1. **Bug alert channel** → untuk notif hanging bug & code review (bug)
2. **SP capacity channel** → untuk notif SP harian per engineer
3. **Code review channel** → untuk notif task engineer yang perlu direview lead

Untuk setiap channel:
1. Klik ⚙️ **Edit Channel** → **Integrations** → **Webhooks**
2. Klik **New Webhook** → beri nama → **Copy Webhook URL**

### 5. Konfigurasi Env Vars

```env
# Jira
JIRA_BASE_URL=https://yourcompany.atlassian.net
JIRA_EMAIL=your-email@company.com
JIRA_API_TOKEN=your_jira_api_token
JIRA_MAX_RESULTS=10

# Menit threshold bug stuck di To Do (default: 10)
BUG_HANGING_MINUTES=10

# Menit threshold bug stuck di Code Review (default: 10)
CODE_REVIEW_HANGING_MINUTES=10

# Discord — Bug alert channel
DISCORD_WEBHOOK_URL=https://discord.com/api/webhooks/xxx/yyy

# Discord user ID lead engineer yang akan di-mention (numeric snowflake)
# Cara dapat: Discord → Settings → Advanced → Enable Developer Mode
#             Lalu klik kanan user → Copy User ID
# Berlaku untuk kedua channel (bug & SP)
DISCORD_LEAD_IDS=123456789012345678,987654321098765432

# Discord — SP capacity alert channel (opsional, SP check dinonaktifkan jika kosong)
DISCORD_SP_ALERT_WEBHOOK_URL=https://discord.com/api/webhooks/xxx/yyy

# Discord — Code review task alert channel (opsional, dinonaktifkan jika kosong)
DISCORD_CODE_REVIEW_WEBHOOK_URL=https://discord.com/api/webhooks/xxx/yyy
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
| `DISCORD_WEBHOOK_URL` | Discord Webhook URL — bug alert channel |
| `DISCORD_LEAD_IDS` | Comma-separated Discord user IDs lead engineer |
| `DISCORD_SP_ALERT_WEBHOOK_URL` | Discord Webhook URL — SP capacity channel |
| `DISCORD_CODE_REVIEW_WEBHOOK_URL` | Discord Webhook URL — code review task alert channel |

### Variables (opsional, ada default)

Tambahkan di **Settings → Secrets and variables → Actions → Variables**:

| Variable | Default | Keterangan |
|----------|---------|------------|
| `JIRA_MAX_RESULTS` | `10` | Jumlah maks hasil query |
| `BUG_HANGING_MINUTES` | `10` | Threshold bug hanging di To Do (menit) |
| `CODE_REVIEW_HANGING_MINUTES` | `10` | Threshold bug hanging di Code Review (menit) |

### Trigger Manual

1. Buka tab **Actions** di GitHub
2. Pilih workflow **🐛 Jira Bug Reminder**
3. Klik **Run workflow**

---

## Cara Kerja

```
startup
  ├── runBugAlerts()        ← langsung dijalankan
  ├── runSPCheck()          ← langsung dijalankan
  └── runCodeReviewCheck()  ← langsung dijalankan

loop:
  ├── setiap 15 menit → runBugAlerts()
  │     ├── cek jam kerja WIB (08:00–18:00), skip jika di luar
  │     ├── GetHangingBugs()        → SendHangingBugAlert()
  │     └── GetHangingCodeReviews() → SendHangingCodeReviewAlert()
  │
  ├── setiap 30 menit → runSPCheck()
  │     ├── cek jam kerja WIB (08:00–18:00), skip jika di luar
  │     ├── GetTasksByExpectedStartDate(today)
  │     │     JQL: issuetype in (Sub-task, "Sub-task Engineer", Subtask, Task)
  │     │           AND "Expected Start Date[Date]" = "YYYY/MM/DD"
  │     │           AND assignee in (<25 engineer names>)
  │     ├── categorizeEngineerSP()
  │     │     → above  : totalSP >= dailyCapacity
  │     │     → below  : totalSP < dailyCapacity
  │     │     → noTasks: tidak ada task hari ini
  │     └── SendSPCapacityAlert()
  │           → 1 summary embed (total SP aktual, kapasitas, utilisasi)
  │           → 1 embed per SPV (✅ sesuai · ⚠️ kurang · 🚫 belum ada task)
  │
  └── setiap 60 menit → runCodeReviewCheck()
        ├── cek jam kerja WIB (08:00–18:00), skip jika di luar
        ├── GetCodeReviewTasks()
        │     JQL: issuetype in (Sub-task, "Sub-task Engineer", Task, Subtask)
        │           AND status in ("CODE REVIEW", "Code Review")
        │           AND created >= "2026/05/04"
        │           AND assignee in (<25 engineer names>)
        │     + fetch changelog per tiket → CodeReviewSince
        └── SendCodeReviewTaskAlert()
              → 1 summary embed (total task, total engineer)
              → 1 embed per SPV (hanya SPV yang punya task Code Review)
```

---

## Contoh Notifikasi Discord

### Hanging Bug Alert — MIDDLE

> `<@lead1> <@lead2>`
>
> ⚙️ **ORANGE ALERT — Bug Menunggu Fixing Engineer**
> Bug dalam fase development telah mencapai batas **MIDDLE** 🟠
>
> 📌 **Epic**
> [PROJ-100] Nama Epic (8 bug)
> https://yourcompany.atlassian.net/browse/PROJ-100
>
> 🦎 **Jumlah Bug (Dev Phase)** &nbsp; 📊 **Threshold**
> **10** bug &nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp; 🔴 High: 15 | 🟠 Mid: 10 | 🟡 Low: 6
>
> 👤 **Breakdown per Assignee**
> **Andi**: 4 Bug | **Budi**: 3 Bug | **Citra**: 2 Bug | **Dian**: 1 Bug
>
> _Eng Ngebug • Development Phase Alert_

---

### SP Capacity Check

> `<@lead1> <@lead2>`
>
> 📊 **SP Capacity Check — 2026-05-07**
> Dari **25** engineer: ✅ **10** sesuai/melebihi target · ⚠️ **8** kurang · 🚫 **7** belum ada task.
>
> 🎯 Total SP Harian (Aktual) &nbsp; 📦 Total Kapasitas SP (Max) &nbsp; 📉 Utilisasi
> **87 SP** dari 42 task &nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp; **154 SP** dari 25 engineer &nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp; **56.5%**
>
> ---
>
> 👤 **SPV: Falih Mulyana** 🔴
> ✅ 1 sesuai · ⚠️ 2 kurang · 🚫 2 belum ada task
>
> **Engineer**
> ✅ **Bayu Kurniawan** — 6 / 6 SP (2 task)
> ⚠️ **Fiqri Ramadhan** — 3 / 6 SP (1 task)
> ⚠️ **Risyadul Alim** — 2 / 6 SP (1 task)
> 🚫 **Muhamad Lutfi Alfiansyah** — belum ada task
> 🚫 **Ridho Tanjung** — belum ada task
>
> _(dst. per SPV...)_

### Code Review Task Alert

> `<@lead1> <@lead2>`
>
> 🔍 **Code Review Needed — 2026-05-07**
> Terdapat **5** task engineer yang sedang dalam status **Code Review** dan membutuhkan review dari lead.
> _Dikelompokkan per SPV._
>
> 📋 **Total Task** &nbsp;&nbsp; 👥 **Total Engineer**
> **5** task &nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp; **3** engineer
>
> _Eng Reminder • Code Review Task Alert_

---

> 👤 **SPV: Faridho**
>
> **Task dalam Code Review**
> 🔸 **Adi Saputra** — [[PROJ-123] Fix data pipeline timeout](https://...atlassian.net/browse/PROJ-123)
>    _2h 15m di Code Review_
> 🔸 **Rizki Gumilar** — [[PROJ-124] Update ETL schema migration](https://...atlassian.net/browse/PROJ-124)
>    _45m di Code Review_
>
> 👤 **SPV: Falih Mulyana**
>
> **Task dalam Code Review**
> 🔸 **Bayu Kurniawan** — [[PROJ-125] Add retry logic for API calls](https://...atlassian.net/browse/PROJ-125)
>    _1h 30m di Code Review_

---

## Format Mention Discord

| Nilai `DISCORD_LEAD_IDS` | Hasil |
|--------------------------|-------|
| `123456789012345678` | `<@123456789012345678>` — mention user spesifik |
| `123456789012345678,987654321098765432` | `<@123...> <@987...>` — multiple user |
| `here` | `@here` — mention semua member online |
| `everyone` | `@everyone` — mention semua member channel |

