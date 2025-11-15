// ============================================
// AUDIO PLAYER MODULE
// Handles live radio and recording playback
// ============================================

import {
  getPlayerState,
  setPlayerType,
  setAudioElement,
  setPlaying,
  setPlayerMetadata,
  resetPlayer,
  getRadioState,
} from "./state.js";

// DOM references (initialized on init)
let mainRadio = null;
let recordingPlayer = null;
let playerFooter = null;
let footerPlayButton = null;
let timeSlider = null;
let currentTimeDisplay = null;
let durationDisplay = null;
let footerTitle = null;
let footerDate = null;
let playBtn = null;
let stopBtn = null;

// ============================================
// SVG ICON CONSTANTS
// ============================================

const PLAY_ICON_SVG = `
  <svg width="24" height="24" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
    <path d="M8 5v14l11-7z" fill="white"/>
  </svg>
`;

const PAUSE_ICON_SVG = `
  <svg width="24" height="24" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
    <path d="M6 19h4V5H6v14zm8-14v14h4V5h-4z" fill="white"/>
  </svg>
`;

// ============================================
// UTILITY FUNCTIONS
// ============================================

function formatTime(seconds) {
  const minutes = Math.floor(seconds / 60);
  const remainingSeconds = Math.floor(seconds % 60);
  return `${minutes}:${remainingSeconds.toString().padStart(2, "0")}`;
}

function updateFooterPlayButton(isPlaying) {
  if (!footerPlayButton) return;

  footerPlayButton.innerHTML = isPlaying
    ? `<svg width="24" height="24" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
            <path d="M6 19h4V5H6v14zm8-14v14h4V5h-4z" fill="black"/>
        </svg>`
    : `<svg width="24" height="24" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
            <path d="M8 5v14l11-7z" fill="black"/>
        </svg>`;
}

// ============================================
// UI UPDATE FUNCTIONS
// ============================================

export function updateMainButtons() {
  if (!playBtn || !stopBtn) return;

  const playerState = getPlayerState();
  if (playerState.type === "live" && playerState.isPlaying) {
    playBtn.style.display = "none";
    stopBtn.style.display = "flex";
  } else {
    playBtn.style.display = "flex";
    stopBtn.style.display = "none";
  }
}

export function updateFooter() {
  if (!playerFooter) return;

  const playerState = getPlayerState();

  if (playerState.isPlaying) {
    playerFooter.classList.add("visible");

    if (playerState.type === "live") {
      // Hide time controls for live radio
      if (timeSlider) timeSlider.style.display = "none";
      if (currentTimeDisplay) currentTimeDisplay.style.display = "none";
      if (durationDisplay) durationDisplay.style.display = "none";
    } else {
      // Show time controls for recordings
      if (timeSlider) timeSlider.style.display = "block";
      if (currentTimeDisplay) currentTimeDisplay.style.display = "block";
      if (durationDisplay) durationDisplay.style.display = "block";
    }

    if (footerTitle) footerTitle.textContent = playerState.metadata.title;
    if (footerDate) footerDate.textContent = playerState.metadata.date;
  } else {
    playerFooter.classList.remove("visible");
  }
}

// ============================================
// LIVE RADIO FUNCTIONS
// ============================================

export function stopRadio() {
  if (!mainRadio) return;

  mainRadio.pause();
  mainRadio.src = "";
  mainRadio.load();

  resetPlayer();
  updateMainButtons();
  updateFooter();
  updateFooterPlayButton(false);

  // Update media session
  if ("mediaSession" in navigator) {
    navigator.mediaSession.playbackState = "paused";
  }
}

export function playRadio() {
  if (!mainRadio) return;

  // Stop recording if playing
  if (recordingPlayer && !recordingPlayer.paused) {
    recordingPlayer.pause();
    recordingPlayer.currentTime = 0;
  }

  // Force fresh stream each time
  let url = "https://radio.cabbage.town:8000/radio.mp3";
  if (navigator.userAgent.includes("Firefox")) {
    url += "?refresh=" + Date.now(); // bust cache
  }

  mainRadio.src = url;
  mainRadio.load();

  mainRadio
    .play()
    .then(() => {
      setPlaying(true);
      setPlayerType("live");
      setAudioElement(mainRadio);

      // Update metadata from current now playing data
      const radioState = getRadioState();
      const nowPlaying = radioState.currentNowPlaying;

      if (nowPlaying) {
        if (nowPlaying.streamerDetails) {
          setPlayerMetadata({
            title: `LIVE: ${nowPlaying.streamerDetails.name} w/ ${nowPlaying.streamerDetails.dj}`,
            date: `${nowPlaying.artist} - ${nowPlaying.title}`,
          });
        } else {
          setPlayerMetadata({
            title: "Live Radio",
            date: `${nowPlaying.artist} - ${nowPlaying.title}`,
          });
        }
      } else {
        setPlayerMetadata({
          title: "Live Radio",
          date: "cabbage.town",
        });
      }

      updateMainButtons();
      updateFooter();
      updateFooterPlayButton(true);

      // Set up media session
      if ("mediaSession" in navigator) {
        const playerState = getPlayerState();
        navigator.mediaSession.metadata = new MediaMetadata({
          title: playerState.metadata.title,
          artist: "cabbage.town",
          artwork: [
            { src: "/the-cabbage.png", sizes: "512x512", type: "image/png" },
          ],
        });

        // Add action handlers
        navigator.mediaSession.setActionHandler("play", () => {
          playRadio();
        });
        navigator.mediaSession.setActionHandler("pause", () => {
          stopRadio();
        });

        navigator.mediaSession.playbackState = "playing";
      }
    })
    .catch((err) => {
      console.warn("playback failed", err);
    });
}

export function updateLiveFooterMetadata(nowPlaying) {
  const playerState = getPlayerState();
  if (playerState.type === "live" && playerState.isPlaying) {
    if (nowPlaying.streamerDetails) {
      setPlayerMetadata({
        title: `LIVE: ${nowPlaying.streamerDetails.name} w/ ${nowPlaying.streamerDetails.dj}`,
        date: `${nowPlaying.artist} - ${nowPlaying.title}`,
      });
    } else {
      setPlayerMetadata({
        title: "Live Radio",
        date: `${nowPlaying.artist} - ${nowPlaying.title}`,
      });
    }

    if (footerTitle) footerTitle.textContent = playerState.metadata.title;
    if (footerDate) footerDate.textContent = playerState.metadata.date;

    // Update media session
    if ("mediaSession" in navigator) {
      navigator.mediaSession.metadata = new MediaMetadata({
        title: playerState.metadata.title,
        artist: "cabbage.town",
        artwork: [
          { src: "/the-cabbage.png", sizes: "512x512", type: "image/png" },
        ],
      });
    }
  }
}

// ============================================
// RECORDING PLAYBACK FUNCTIONS
// ============================================

// Track which button is currently active for a recording
let currentRecordingButton = null;
let currentRecording = null;

export function playRecording(show, playButton) {
  // If clicking the same recording that's playing, pause it
  if (
    currentRecording?.recording?.url === show.recording.url &&
    getPlayerState().isPlaying &&
    getPlayerState().type === "recording"
  ) {
    recordingPlayer.pause();
    return;
  }

  // Stop live radio if playing
  const playerState = getPlayerState();
  if (playerState.type === "live" && playerState.isPlaying) {
    stopRadio();
  }

  // Update previous button to play icon
  if (currentRecordingButton && currentRecordingButton !== playButton) {
    currentRecordingButton.innerHTML = PLAY_ICON_SVG;
    currentRecordingButton.style.backgroundColor = "black";
    currentRecordingButton.style.opacity = "1";
  }

  // Set current recording and button
  currentRecording = show;
  currentRecordingButton = playButton;

  // Load new source
  recordingPlayer.src = show.recording.url;
  recordingPlayer.load();

  // Show loading state
  playButton.style.opacity = "0.5";

  // Wait for load then play
  recordingPlayer.onloadeddata = () => {
    playButton.style.opacity = "1";
    recordingPlayer.play();
  };

  recordingPlayer.onerror = () => {
    playButton.style.opacity = "1";
    playButton.style.backgroundColor = "#d32f2f";
    console.error("Failed to load recording:", show.recording.url);
  };
}

export function stopRecording() {
  if (!recordingPlayer) return;

  recordingPlayer.pause();
  recordingPlayer.currentTime = 0;

  if (currentRecordingButton) {
    currentRecordingButton.innerHTML = PLAY_ICON_SVG;
    currentRecordingButton.style.backgroundColor = "black";
  }

  resetPlayer();
  updateMainButtons();
  updateFooter();
  updateFooterPlayButton(false);
}

// Set up recording player event handlers (called from initPlayer)
function setupRecordingPlayer() {
  if (!recordingPlayer) return;

  recordingPlayer.onplay = () => {
    setPlaying(true);
    setPlayerType("recording");
    setAudioElement(recordingPlayer);
    setPlayerMetadata({
      title: currentRecording.title,
      date: currentRecording.date,
    });

    updateMainButtons();
    updateFooter();
    updateFooterPlayButton(true);

    // Update current button to pause icon
    if (currentRecordingButton) {
      currentRecordingButton.innerHTML = PAUSE_ICON_SVG;
    }

    if ("mediaSession" in navigator) {
      navigator.mediaSession.metadata = new MediaMetadata({
        title: currentRecording.title,
        artist: currentRecording.author || "cabbage.town",
        artwork: [
          { src: "/the-cabbage.png", sizes: "512x512", type: "image/png" },
        ],
      });
      navigator.mediaSession.playbackState = "playing";
    }
  };

  recordingPlayer.onpause = () => {
    const playerState = getPlayerState();
    if (playerState.audioElement === recordingPlayer) {
      setPlaying(false);
      updateFooterPlayButton(false);
      if ("mediaSession" in navigator) {
        navigator.mediaSession.playbackState = "paused";
      }
    }

    // Update current button to play icon
    if (currentRecordingButton) {
      currentRecordingButton.innerHTML = PLAY_ICON_SVG;
    }
  };

  recordingPlayer.onloadedmetadata = () => {
    const playerState = getPlayerState();
    if (playerState.audioElement === recordingPlayer) {
      if (timeSlider) timeSlider.max = recordingPlayer.duration;
      if (durationDisplay)
        durationDisplay.textContent = formatTime(recordingPlayer.duration);
    }
  };

  recordingPlayer.ontimeupdate = () => {
    const playerState = getPlayerState();
    if (
      playerState.audioElement === recordingPlayer &&
      timeSlider &&
      !timeSlider.dragging
    ) {
      timeSlider.value = recordingPlayer.currentTime;
      if (currentTimeDisplay)
        currentTimeDisplay.textContent = formatTime(
          recordingPlayer.currentTime
        );
    }
  };

  recordingPlayer.onended = () => {
    const playerState = getPlayerState();
    if (playerState.audioElement === recordingPlayer) {
      resetPlayer();
      updateFooter();
      updateFooterPlayButton(false);

      if (currentRecordingButton) {
        currentRecordingButton.innerHTML = PLAY_ICON_SVG;
      }
    }
  };
}

// ============================================
// INITIALIZATION
// ============================================

export function initPlayer() {
  console.log("[Player] Initializing player module");

  // Get DOM references
  mainRadio = document.getElementById("radio");
  recordingPlayer = document.getElementById("recordingPlayer");
  playerFooter = document.getElementById("playerFooter");
  footerPlayButton = document.getElementById("footerPlayButton");
  timeSlider = document.getElementById("timeSlider");
  currentTimeDisplay = document.getElementById("currentTime");
  durationDisplay = document.getElementById("duration");
  footerTitle = document.getElementById("footerTitle");
  footerDate = document.getElementById("footerDate");
  playBtn = document.getElementById("play");
  stopBtn = document.getElementById("stop");

  // Wire up live radio controls if they exist
  if (playBtn) {
    playBtn.onclick = () => {
      const playerState = getPlayerState();
      if (playerState.isPlaying && playerState.type === "live") {
        stopRadio();
      } else {
        playRadio();
      }
    };
  }

  if (stopBtn) {
    stopBtn.onclick = () => {
      stopRadio();
    };
  }

  if (mainRadio) {
    mainRadio.onerror = (e) => {
      const err = mainRadio.error;
      if (err?.code === MediaError.MEDIA_ERR_NETWORK) {
        console.log("stream error, retrying in 5s...");
        setTimeout(playRadio, 5000);
      }
    };

    mainRadio.onended = stopRadio;
  }

  // Set up recording player event handlers
  setupRecordingPlayer();

  // Footer play button controls both live and recording playback
  if (footerPlayButton) {
    footerPlayButton.onclick = () => {
      const playerState = getPlayerState();
      if (playerState.type === "live") {
        if (playerState.isPlaying) {
          stopRadio();
        } else {
          playRadio();
        }
      } else if (playerState.type === "recording") {
        if (playerState.isPlaying) {
          recordingPlayer.pause();
        } else {
          recordingPlayer.play();
        }
      }
    };
  }

  // Time slider controls for recordings
  if (timeSlider) {
    let isTimeSliderDragging = false;
    timeSlider.addEventListener("mousedown", () => {
      isTimeSliderDragging = true;
    });

    timeSlider.addEventListener("mouseup", () => {
      isTimeSliderDragging = false;
      const playerState = getPlayerState();
      if (playerState.type === "recording" && playerState.audioElement) {
        recordingPlayer.currentTime = timeSlider.value;
      }
    });
  }

  console.log("[Player] Player module initialized");
}
