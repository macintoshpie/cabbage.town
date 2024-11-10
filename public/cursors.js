const cursors = [
  "url(cursor-tnt-0.png) 3 0, auto",
  "url(cursor-paint-brush.png) 15 15, auto",
  "url(cursor-pencil.png) 5 12, auto",
  "url(tool-ptr-bucket-fill.png) 18 20, auto",
];
let randomCursor = Math.floor(Math.random() * cursors.length);
const body = document.querySelector("body");
body.style.cursor = cursors[randomCursor];
