const cursors = [
  "url(cursor-tnt-0.png) 3 0, auto",
  "url(cursor-paint-brush.png) 15 15, auto",
  "url(cursor-pencil.png) 5 12, auto",
  "url(tool-ptr-bucket-fill.png) 18 20, auto",
  "url(cursor-lobster.cur), auto",
  "url(cursor-scimi.cur), auto",
];

let currentCursorIndex = Math.floor(Math.random() * cursors.length);

document.getElementById("logo").addEventListener("click", function () {
  console.log("clicked");
  currentCursorIndex = (currentCursorIndex + 1) % cursors.length;
  document.body.style.cursor = cursors[currentCursorIndex];
});
