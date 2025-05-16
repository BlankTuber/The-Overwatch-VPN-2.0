# 🎮 Overwatch VPN 2.0 🌐

A super-cool Go-based application that helps Overwatch players take control of their gaming destiny! Say goodbye to high ping and laggy matches by choosing exactly which region servers you connect to! ✨

## 🧩 Components

This awesome project consists of three magical components:

1. **IP Puller** 🔍 - Hunts down and captures Overwatch server IPs by region from GitHub
2. **Firewall Sidecar** 🛡️ - Your personal bouncer that tells unwanted server connections to get lost
3. **Fyne GUI** 🖥️ - Pretty buttons and lights that make the magic happen with just a click!

## ⚡ How It Works

### 🚀 Startup Sequence

1. Launch the app and watch as it fetches the freshest Overwatch server IPs available 📡
2. The IP Puller sorts these IPs by region and tucks them neatly into text files 📋
3. Firewall Sidecar springs into action (with a quick UAC prompt - we need those superpowers!) 💪
4. The colorful GUI appears, ready for you to decide which regions deserve your presence! 🎭

### 🎯 Runtime Behavior

- Block a region and POOF! 💨 Firewall rules appear, specifically tailored for your Overwatch.exe
- Is Overwatch running? The app will politely ask you to close it first (it can't work magic while you're mid-match!) ⚠️
- Unblocking works instantly - even if you're in-game! 🏎️
- The app keeps a watchful eye on Overwatch and updates the UI faster than you can say "Nerf this!" 👀
- Automatically detects where your Overwatch lives on your PC when you launch the game 🔮

### 🔄 Shutdown Sequence

- When you close the app, it tidies up after itself: 🧹
  1. Unblocks all previously blocked IPs ✅
  2. Sweeps away all those firewall rules 🧽
  3. Waves goodbye and shuts down all components with grace and style 👋

## 📁 Project Structure

```
Overwatch-VPN/
├── Ip-Puller/                    # 🔍 IP hunting ground
│   ├── cmd/puller/main.go        # 🚪 Entry point 
│   ├── internal/github/github.go # 🐙 GitHub connection
│   ├── internal/regions/regions.go # 🗺️ Region sorting magic
│   └── internal/output/output.go # 💾 File saving wizardry
│
├── firewall-interaction/         # 🛡️ Firewall control center
│   ├── cmd/sidecar/main.go       # 🚪 Entry point
│   ├── internal/firewall/firewall.go # 🧱 Windows firewall tamer
│   ├── internal/process/process.go # 👀 Process spy
│   └── internal/config/config.go   # ⚙️ Configuration stuff
│
├── fyne-gui/                     # 🎨 Pretty interface
│   ├── main.go                   # 🎭 Where the UI magic happens
│   ├── go.mod                    # 📋 Go module requirements
│   └── go.sum                    # 🧮 Dependencies checksum
│
├── installer/                    # 📦 Package wrapper
│   └── installer.iss             # 🔧 InnoSetup script
│
└── build.bat                     # 🏗️ Builder script (one click to rule them all!)
```

## 📋 Requirements

- Windows 10 or later (sorry Mac & Linux folks! 🍎🐧)
- Administrator privileges (we need the keys to the kingdom! 👑)
- Overwatch installed (duh! 😉)

## 🏗️ Building

Want to build it yourself? You tech wizard! Just run:

```bash
# The magic incantation:
build.bat
```

This will make all sorts of exciting things happen:
1. Build the IP Puller 🔍
2. Craft the Firewall Sidecar with special admin powers 🛡️
3. Conjure up the Fyne GUI application 🎨
4. Place all the goodies in the bin directory 📦

## 🎮 Usage

1. Launch the app (it'll ask for admin rights - say yes! Trust us! 😇)
2. The app will sit patiently waiting to detect Overwatch when you run it 🔍
3. Pick the regions full of players who make you sad and BLOCK THEM! 🚫
4. Launch Overwatch and enjoy the sweet, sweet taste of low ping! 🍯

## ⚠️ Important Notes

- Blocking won't work if Overwatch is running (the app will give you puppy dog eyes until you close it) 🐶
- You might need to restart Overwatch after changing settings (it needs a moment to process its feelings) 😢
- All blocks vanish when you close the app (we leave no trace behind, like digital ninjas!) 🥷
- The app will automatically find your Overwatch executable faster than a Tracer can blink! ⚡

## 🔧 Technical Details

### 🔍 IP Puller

- Grabs delicious IP ranges from the foryVERX/Overwatch-Server-Selector GitHub repository 🍽️
- Sorts IPs by region like a very specific trading card collection (EU, NA, AS, AFR, ME, OCE, SA) 🃏
- Updates IP lists when newer versions appear (always staying fresh!) 🌱

### 🛡️ Firewall Sidecar

- Creates firewall rules that would make any network admin proud 👔
- Speaks the ancient language of Windows Firewall with Advanced Security API 📜
- Processes IP ranges in batches because it's efficient (and likes to show off) 🏅
- Needs admin rights (it's kind of a big deal) 💼
- Whispers to the GUI through mysterious stdin/stdout pipes 🧙‍♂️

### 🎨 Fyne GUI

- Built with Fyne, a Go UI toolkit that makes things pretty 🦋
- Gives you buttons that are just begging to be clicked 👆
- Watches Overwatch like a stalker (but the good kind!) 👁️
- Shows you all the juicy connection details and logs 📊
- Includes a help section for when you're feeling lost and confused (we've all been there) 🤔

## 📜 License

GNU General Public License v3.0 (Free as in freedom! 🗽)

## 🙏 Acknowledgements

- [Fyne](https://fyne.io/) - For making our GUI look fabulous! 💅
- [foryVERX/Overwatch-Server-Selector](https://github.com/foryVERX/Overwatch-Server-Selector) - For the IP data that makes our dreams come true! 💭
