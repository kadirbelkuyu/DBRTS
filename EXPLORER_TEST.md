# Explorer Test Guide

## Sorun
Explorer modu (5. seçenek) seçildiğinde siyah ekranda kalma sorunu çözüldü.

## Yapılan İyileştirmeler

### 1. Bağlantı Durumu Gösterimi
- MongoDB/PostgreSQL bağlantısı öncesi bilgilendirme mesajları
- Bağlantı parametrelerinin gösterimi
- Şifrelerin maskelenmesi

### 2. Hata Yönetimi
- TUI başlatma hatalarının yakalanması
- Detaylı hata mesajları
- Kullanıcı dostu geri bildirimler

## Test Adımları

### Seçenek 1: Komut Satırından Direkt Test

```bash
./bin/dbrts explore --config configs/mongo-local.yaml
```

### Seçenek 2: İnteraktif Moddan Test

```bash
./bin/dbrts
# 5) Explore a database with the TUI seçin
# 2) mongo-local (mongo) profilini seçin
```

### Seçenek 3: Test Script ile

```bash
./test-explorer.sh
```

## Beklenen Çıktı

### Başarılı Bağlantı
```
Connecting to MongoDB...
URI: mongodb://***:***@localhost:27017/admin?directConnection=true
Database: admin

Connected successfully! Loading explorer...
Starting TUI... (Press 'q' to exit)
```

Sonrasında:
- Sol tarafta Collections listesi görünecek
- Sağ tarafta seçili collection'ın dokümanları görünecek
- 'q' tuşu ile çıkış yapabilirsiniz

### Başarısız Bağlantı
```
Connecting to MongoDB...
URI: mongodb://***:***@localhost:27017/admin?directConnection=true
Database: admin

failed to connect to MongoDB: ...detaylı hata mesajı...
```

## MongoDB'nin Çalıştığından Emin Olun

Eğer MongoDB localhost'ta çalışmıyorsa:

```bash
# Docker ile MongoDB başlatma
docker run -d -p 27017:27017 \
  -e MONGO_INITDB_ROOT_USERNAME=root \
  -e MONGO_INITDB_ROOT_PASSWORD=f2Hl0HyGnJ \
  mongo:latest
```

## Sorun Giderme

### Hala Siyah Ekranda Kalıyorsa

1. Terminal emülatörünüzü kontrol edin (iTerm2, Terminal.app vb.)
2. `TERM` environment variable'ını kontrol edin:
   ```bash
   echo $TERM
   ```
3. Eğer sorun devam ederse, debug modunda çalıştırın:
   ```bash
   GODEBUG=netdns=go ./bin/dbrts explore --config configs/mongo-local.yaml 2>&1 | tee debug.log
   ```

### MongoDB Bağlantı Sorunları

- MongoDB servisinin çalıştığından emin olun
- Firewall ayarlarını kontrol edin
- Doğru kullanıcı adı ve şifre kullandığınızdan emin olun

## Config Dosyaları

Proje içinde hazır config dosyaları:

- `configs/mongo-local.yaml` - Localhost MongoDB
- `configs/source-mongo.yaml` - Remote MongoDB (host based)
- `configs/test-mongo.yaml` - MongoDB Atlas/Digital Ocean
- `configs/postgres-dev.yaml` - Dev PostgreSQL
- `configs/postgres-stage.yaml` - Stage PostgreSQL

