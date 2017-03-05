(function search() {
  var existingPages = document.getElementById("existingPages");
  var searchForm = document.getElementById("searchForm");
  var pageNameInput = document.getElementById("pageNameInput");

  if (!isAdmin) {
    pageNameInput.addEventListener("focus", function (event) {
      searchForm.className = "";
    });
  }

  searchForm.addEventListener("submit", function (event) {
    event.preventDefault();
    var pageName = searchForm.pageName.value;
    var pageID = pageName.replace(" ", "-");
    if (pageName === "") {
      return;
    }

    for (var i = 0; i < existingPages.children.length; i++) {
      if (existingPages.children[i].value === pageName) {
        location.assign("/view/" + pageID);
        return;
      }
    }

    if (isAdmin) {
      location.assign("/edit/" + pageID);
      return;
    }
    
    searchForm.className = "noSuchPage";
  });

})();