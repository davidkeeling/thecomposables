(function initVersionSwitcher() {
  var versionSelector = document.getElementById("versionSelector");
  var contentContainer = document.getElementById("contentContainer");
  if (versionSelector === null) return;
  versionSelector.addEventListener("change", function (event) {
    var version = event.target.value;
    var markup;
    if (version === "") {
      markup = currentVersion;
    } else {
      markup = versions[Number(version)].Markup;
    }
    contentContainer.innerHTML = markup;
  }, false);
})();