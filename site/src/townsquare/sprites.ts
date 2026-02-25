// Tiny pixel-art cabbage character (12x14 pixels)
// Each row is an array of hex color strings or '' for transparent

const _ = '';  // transparent
const G = '#5a9e3e'; // green (outer leaves)
const L = '#7ec850'; // light green (inner leaves)
const D = '#3d7a2a'; // dark green (leaf shadows)
const B = '#f5e6c8'; // body/face
const F = '#e8d4a8'; // face shadow
const E = '#2d2d2d'; // eyes
const M = '#c45c4a'; // mouth/cheeks
const S = '#4a8530'; // stem

export const CABBAGE_SPRITE: (string | '')[][] = [
  //0  1  2  3  4  5  6  7  8  9  10 11
  [ _, _, _, _, S, S, S, S, _, _, _,  _],  // 0  stem
  [ _, _, _, D, G, G, G, G, D, _, _,  _],  // 1  top leaves
  [ _, _, D, G, L, L, L, L, G, D, _,  _],  // 2
  [ _, D, G, L, L, G, G, L, L, G, D,  _],  // 3  leaf detail
  [ _, G, L, L, G, L, L, G, L, L, G,  _],  // 4
  [ D, G, L, B, B, B, B, B, B, L, G,  D],  // 5  face top
  [ D, G, B, B, E, B, B, E, B, B, G,  D],  // 6  eyes
  [ D, G, B, B, B, B, B, B, B, B, G,  D],  // 7
  [ _, G, B, M, B, B, B, B, M, B, G,  _],  // 8  cheeks
  [ _, G, B, B, B, M, M, B, B, B, G,  _],  // 9  mouth
  [ _, D, G, B, B, B, B, B, B, G, D,  _],  // 10 face bottom
  [ _, _, D, G, F, F, F, F, G, D, _,  _],  // 11 chin
  [ _, _, _, D, G, G, G, G, D, _, _,  _],  // 12 bottom leaves
  [ _, _, _, _, D, D, D, D, _, _, _,  _],  // 13 shadow
];

export const SPRITE_W = CABBAGE_SPRITE[0].length; // 12
export const SPRITE_H = CABBAGE_SPRITE.length;     // 14
