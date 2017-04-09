var bodyInput = document.querySelector('[name="body"]');
document.addEventListener('keypress', handleKeyPress);

function handleKeyPress(event) {
  if (event.ctrlKey && event.shiftKey && event.keyCode == 12) linkTransform();
}

function linkTransform() {
  var start = bodyInput.selectionStart;
  var end = bodyInput.selectionEnd;
  var text = bodyInput.value;
  var before = text.slice(0, start);
  var after = text.slice(end);
  var selection = text.slice(start, end).trim();
  if (!selection) return;

  var replacement = fmt('[{0}](/view/{1})', selection, selection.replace(' ', '-'));
  if (text[end - 1] === ' ') replacement += ' ';
  bodyInput.value = before + replacement + after;
  bodyInput.setSelectionRange(start + replacement.length, start + replacement.length);
}

function fmt(str) {
  var args = arguments;
  return str.replace(/{(\d+)}/g, function (match, num) { 
    var index = Number(num) + 1;
    return typeof args[index] !== 'undefined' ? args[index] : match;
  });
}
