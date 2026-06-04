chrome.commands.onCommand.addListener((command) => {
  if (command === "extract-token") {
    chrome.action.openPopup();
  }
});
