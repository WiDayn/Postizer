const settingsStatus = document.querySelector("#settingsStatus");
const homeImageUpload = document.querySelector("#homeImageUpload");
const homeImageInput = document.querySelector("#homeImageInput");
const clearHomeImageButton = document.querySelector("#clearHomeImageButton");

function setSettingsStatus(message) {
  if (settingsStatus) settingsStatus.textContent = message;
}

if (homeImageUpload && homeImageInput) {
  homeImageUpload.addEventListener("submit", async (event) => {
    event.preventDefault();
    const file = homeImageInput.files && homeImageInput.files[0];
    if (!file) return;
    const data = new FormData();
    data.append("file", file, file.name || "home-image.webp");
    setSettingsStatus("Uploading");
    const response = await fetch("/admin/api/home-image", { method: "POST", body: data });
    if (!response.ok) {
      setSettingsStatus((await response.text()).trim());
      return;
    }
    setSettingsStatus("Saved");
    window.location.reload();
  });
}

if (clearHomeImageButton) {
  clearHomeImageButton.addEventListener("click", async () => {
    setSettingsStatus("Clearing");
    const response = await fetch("/admin/api/home-image", { method: "DELETE" });
    if (!response.ok) {
      setSettingsStatus((await response.text()).trim());
      return;
    }
    setSettingsStatus("Cleared");
    window.location.reload();
  });
}
