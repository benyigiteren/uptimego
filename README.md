# 🚀 UptimeGo

UptimeGo, web servislerinizin ve sunucularınızın erişilebilirliğini (HTTP ve Ping tabanlı) anlık olarak takip eden, servis kesintilerinde Telegram, Discord ve E-posta (SMTP) üzerinden akıllı bildirimler gönderen, modern ve son derece hafif (boşta < 15MB RAM tüketen) bir **Uptime & Status** yönetim panelidir.

Uygulama, hem bireysel projelerinizi hem de kurumsal sistemlerinizi izlemek için **Go** dili ile minimalist, sıfır bağımlılıklı (Pure Go SQLite) ve taşınabilir şekilde geliştirilmiştir.

---

## ✨ Özellikler

- ⏱️ **Gelişmiş Servis Takibi**: HTTP(S) istekleri (anahtar kelime kontrolü, SSL sertifikası son gün uyarısı) ve ICMP Ping protokolü ile kesintisiz izleme.
- 📢 **Çoklu Bildirim Kanalları**: SMTP (E-posta), Telegram (Bot API) ve Discord Webhook entegrasyonu.
- ⚡ **Akıllı Bildirim Şablonları**: Tamamen özelleştirilebilir, emojili ve profesyonel Türkçe varsayılan bildirim mesajları.
- 🌐 **Halk Açık Durum Sayfası (Public Status Page)**: Ziyaretçileriniz için servisin genel durumunu, anlık gecikmelerini ve geçmiş loglarını gösteren şık bir arayüz.
- 👤 **Rol Tabanlı Yetkilendirme (RBAC)**: `Super Admin`, `Admin` ve `Viewer` rolleri ile güvenlik katmanı.
- 🔑 **Gelişmiş REST API (V1)**: API anahtarınızla dış sistemlere entegre edin (Monitör ekleme, silme, listeleme).
- 🎨 **Premium Arayüz & Animasyonlar**: Alpine.js ve Tailwind CSS ile geliştirilmiş modern, responsive ve akıcı cam efekti (glassmorphism) tasarımlı karanlık mod arayüzü.
- 💾 **Otomatik Kaydetme (Auto-Save)**: Ayarlar sayfasındaki tüm form girişleri (profil şifre güncelleme hariç) kullanıcı yazmayı bitirdiğinde (~800ms gecikmeyle) otomatik olarak veritabanına kaydedilir.
- 🗄️ **Sıfır Bağımlılık & Taşınabilir Veritabanı**: SQLite WAL modu ile yüksek performans ve kolay yedekleme. 30 günlük logları otomatik temizleyen arka plan aracı.
- 🌐 **Çoklu Dil Desteği**: Türkçe (TR) ve İngilizce (EN) arayüz desteği.

---

## 🛠️ Kurulum ve Çalıştırma

UptimeGo'yu Docker kullanarak veya doğrudan Go binary ile kolayca ayağa kaldırabilirsiniz.

### 🐳 1. Docker Compose ile Kurulum (Önerilen)

En kolay ve hızlı kurulum yöntemi Docker Compose kullanmaktır.

1. **`docker-compose.yml` dosyasını oluşturun:**
   ```yaml
   version: '3.8'

   services:
     uptimego:
       image: benyigiteren/uptimego:latest # veya local build için build: .
       container_name: uptimego
       restart: always
       ports:
         - "8080:8080"
       volumes:
         - ./data:/app/data
       environment:
         - TZ=Europe/Istanbul
       cap_add:
         - NET_RAW # ICMP Ping paketleri için gereklidir
   ```

2. **Konteyneri başlatın:**
   ```bash
   docker-compose up -d
   ```
   Uygulama `http://localhost:8080` adresinde çalışmaya başlayacaktır.

---

### 🏗️ 2. Kaynak Koddan Derleme (Local Build)

Bilgisayarınızda Go (1.22 veya üzeri) kuruluysa projeyi doğrudan derleyebilirsiniz.

1. **Projeyi klonlayın ve dizine gidin:**
   ```bash
   git clone https://github.com/benyigiteren/uptimego.git
   cd uptimego
   ```

2. **Projeyi derleyin:**
   - **Linux / macOS:**
     ```bash
     go build -o uptimego main.go
     ```
   - **Windows:**
     ```powershell
     go build -o uptimego.exe main.go
     ```

3. **Uygulamayı çalıştırın:**
   ```bash
   ./uptimego -port 8080 -db data/uptimego.db
   ```

---

## 🚀 İlk Kurulum (Setup)

Uygulamayı ilk başlattığınızda tarayıcınızdan `http://localhost:8080` adresine gittiğinizde otomatik olarak kurulum ekranına yönlendirilirsiniz.

1. Super Admin için güvenli bir **kullanıcı adı** ve **şifre** belirleyin.
2. Kurulumu tamamladıktan sonra giriş yaparak kontrol paneline ulaşabilirsiniz.
3. Giriş yaptıktan sonra **Ayarlar** sekmesine giderek SMTP, Telegram ve Discord bildirim kanallarını yapılandırabilirsiniz.

---

## 🔑 REST API V1 Kullanımı

UptimeGo, harici yazılımlarla entegrasyon için basit ve güvenli bir REST API sunar. İsteklerinize HTTP başlığında `X-API-Key: <API_ANAHTARINIZ>` bilgisini eklemeniz gerekir. API anahtarınızı ayarlar sayfasından alabilir veya sıfırlayabilirsiniz.

### 📋 1. Monitörleri Listeleme
```bash
curl -H "X-API-Key: API_ANAHTARINIZ" http://localhost:8080/api/v1/monitors
```

### ➕ 2. Yeni Monitör Ekleme
```bash
curl -X POST \
  -H "Content-Type: application/json" \
  -H "X-API-Key: API_ANAHTARINIZ" \
  -d '{
    "name": "Benim Web Sitem",
    "type": "http",
    "target": "https://google.com",
    "interval": 60,
    "timeout": 2000,
    "retries": 3,
    "alert_interval": 0,
    "active": true,
    "public": true
  }' \
  http://localhost:8080/api/v1/monitors
```

### ❌ 3. Monitör Silme
```bash
curl -X DELETE -H "X-API-Key: API_ANAHTARINIZ" "http://localhost:8080/api/v1/monitors?id=MONITOR_ID"
```

---

## 💾 RAM ve Performans Optimizasyonları

UptimeGo, sistem kaynaklarını en az düzeyde tüketmek üzere tasarlanmıştır:
- Go Runtime Garbage Collection agresif çalışacak şekilde (`GOGC=15`) ayarlanmıştır.
- Kullanılmayan boş bellek sayfaları her 2 dakikada bir işletim sistemine otomatik olarak geri iade edilir (`FreeOSMemory`).
- SQLite veritabanı cache'i ~1MB ile sınırlandırılmıştır (`cache_size=-1000`) ve log temizleme sonrasında bellek boşaltması (`shrink_memory`) tetiklenir.
- Bu optimizasyonlar sayesinde boşta RAM kullanımı **~8-12 MB** arasında kalmaktadır.

---

## 🤝 Katkıda Bulunma

1. Bu depoyu çatallayın (Fork).
2. Yeni bir özellik dalı (Branch) oluşturun (`git checkout -b ozellik/yeni-ozellik`).
3. Değişikliklerinizi taahhüt edin (Commit) (`git commit -m 'Yeni özellik eklendi'`).
4. Dalınızı gönderin (Push) (`git push origin ozellik/yeni-ozellik`).
5. Bir Çekme İsteği (Pull Request) oluşturun.

---

## 📄 Lisans

Bu proje **MIT Lisansı** ile lisanslanmıştır. Daha fazla bilgi için `LICENSE` dosyasına göz atabilirsiniz.
