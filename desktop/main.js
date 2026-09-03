const { app, BrowserWindow, ipcMain, safeStorage } = require("electron");
const fs = require("fs");
const path = require("path");

function createWindow() {
  const win = new BrowserWindow({
    width: 1200,
    height: 800,
    minWidth: 900,
    minHeight: 640,
    icon: path.join(__dirname, "icon.png"),
    webPreferences: {
      preload: path.join(__dirname, "preload.js"),
      contextIsolation: true,
      nodeIntegration: false,
    },
  });
  win.loadFile(path.join(__dirname, "index.html"));
}

function tokenFilePath() {
  return path.join(app.getPath("userData"), "secure-token.bin");
}

ipcMain.handle("secureToken:get", async () => {
  try {
    const fp = tokenFilePath();
    if (!fs.existsSync(fp)) return "";
    const raw = fs.readFileSync(fp);
    if (!raw || raw.length === 0) return "";
    if (safeStorage.isEncryptionAvailable()) {
      return safeStorage.decryptString(raw);
    }
    return raw.toString("utf8");
  } catch {
    return "";
  }
});

ipcMain.handle("secureToken:set", async (_e, value) => {
  try {
    const token = typeof value === "string" ? value : "";
    const fp = tokenFilePath();
    if (!token) {
      if (fs.existsSync(fp)) fs.unlinkSync(fp);
      return true;
    }
    const raw = safeStorage.isEncryptionAvailable() ? safeStorage.encryptString(token) : Buffer.from(token, "utf8");
    fs.writeFileSync(fp, raw);
    return true;
  } catch {
    return false;
  }
});

app.whenReady().then(() => {
  app.setName("NetLynx");
  createWindow();
  app.on("activate", () => {
    if (BrowserWindow.getAllWindows().length === 0) {
      createWindow();
    }
  });
});

app.on("window-all-closed", () => {
  if (process.platform !== "darwin") {
    app.quit();
  }
});
