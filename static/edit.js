var bodyInput = document.querySelector('[name="body"]');
document.addEventListener('keypress', function (event) {
  if (!event.ctrlKey || !event.shiftKey || event.keyCode !== 12) return;
  var start = bodyInput.selectionStart;
  var end = bodyInput.selectionEnd;
  var text = bodyInput.value;
  var before = text.slice(0, start);
  var after = text.slice(end);
  var selection = text.slice(start, end).trim();
  if (!selection) return;

  var replacement = '[' + selection + '](/view/' + selection.replace(' ', '-') + ')';
  if (text[end - 1] === ' ') replacement += ' ';
  bodyInput.value = before + replacement + after;
  bodyInput.setSelectionRange(start + replacement.length, start + replacement.length);
});
