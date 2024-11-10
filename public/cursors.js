const cursors = [
  "url(cursor-tnt-0.png) 3 0, auto",
  "url(cursor-paint-brush.png) 15 15, auto",
  "url(cursor-pencil.png) 5 12, auto",
  "url(tool-ptr-bucket-fill.png) 18 20, auto",
];
let currentCursor = 0;
const body = document.querySelector("body");
body.addEventListener("click", () => {
  currentCursor = (currentCursor + 1) % cursors.length;
  body.style.cursor = cursors[currentCursor];
});
