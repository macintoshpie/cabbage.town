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

let rotationDegrees = 0;
document.getElementById("logo").addEventListener("click", function () {
  rotationDegrees += 45; // Rotate 45 degrees each click
  this.style.transform = `rotate(${rotationDegrees}deg)`;
  this.style.transition = "transform 0.3s ease";
});

document.getElementById("logo").addEventListener("click", function () {
  const firework = document.createElement("img");
  firework.src = Math.random() > 0.5 ? "firework-1.gif" : "firework-2.gif";
  firework.style.cssText = `
        position: fixed;
        width: 100px;
        height: 100px;
        left: ${Math.random() * window.innerWidth}px;
        top: ${Math.random() * window.innerHeight}px;
        pointer-events: none;
        z-index: 1000;
    `;
  document.body.appendChild(firework);
  setTimeout(() => firework.remove(), 1000); // Remove after animation
});

let isKidPixMode = false;
document.getElementById("logo").addEventListener("click", function () {
  isKidPixMode = !isKidPixMode;
  document.body.style.backgroundImage = isKidPixMode
    ? "url(kidpix-cabbagetown-cropped.png)"
    : "none";
  document.body.style.backgroundColor = isKidPixMode
    ? "transparent"
    : "var(--dayellow)";
});

document.getElementById("logo").addEventListener("click", function () {
  const leftLight = document.createElement("img");
  const rightLight = document.createElement("img");
  leftLight.src = "leftlight.gif";
  rightLight.src = "rightlight.gif";

  [leftLight, rightLight].forEach((light, index) => {
    light.style.cssText = `
            position: fixed;
            width: 200px;
            height: 200px;
            ${index === 0 ? "left: 0;" : "right: 0;"}
            top: 50%;
            transform: translateY(-50%);
            pointer-events: none;
            z-index: 1000;
        `;
    document.body.appendChild(light);
    setTimeout(() => light.remove(), 2000);
  });
});
