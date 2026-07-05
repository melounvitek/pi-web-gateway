const { contextBridge, ipcRenderer } = require("electron");

contextBridge.exposeInMainWorld("piGatewayDesktop", {
  activateGateway: (id) => ipcRenderer.invoke("gateway-config:activate", id),
  addGateway: (gateway) => ipcRenderer.invoke("gateway-config:add-gateway", gateway),
  getGatewayConfig: () => ipcRenderer.invoke("gateway-config:get"),
  onAddGatewayRequested: (callback) => {
    ipcRenderer.on("gateway:add-requested", callback);
  },
  saveGateway: (gateway) => ipcRenderer.invoke("gateway-config:save-gateway", gateway)
});
