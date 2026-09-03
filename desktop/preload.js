const { contextBridge, ipcRenderer } = require("electron");

contextBridge.exposeInMainWorld("invetorDesktop", {
  version: "0.1.0",
  secureTokenGet: () => ipcRenderer.invoke("secureToken:get"),
  secureTokenSet: (value) => ipcRenderer.invoke("secureToken:set", value),
});
