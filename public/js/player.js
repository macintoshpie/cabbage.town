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
    getRadioState
} from './state.js';

// DOM references (initialized on init)
let mainRadio = null;
let playerFooter = null;
let footerPlayButton = null;
let timeSlider = null;
let currentTimeDisplay = null;
let durationDisplay = null;
let footerTitle = null;
let footerDate = null;
let playBtn = null;
let stopBtn = null;

// Recording players array
const recordingPlayers = [];

// ============================================
// UTILITY FUNCTIONS
// ============================================

function formatTime(seconds) {
    const minutes = Math.floor(seconds / 60);
    const remainingSeconds = Math.floor(seconds % 60);
    return `${minutes}:${remainingSeconds.toString().padStart(2, '0')}`;
}

function updateFooterPlayButton(isPlaying) {
    if (!footerPlayButton) return;
    
    footerPlayButton.innerHTML = isPlaying ?
        `<svg width="24" height="24" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
            <path d="M6 19h4V5H6v14zm8-14v14h4V5h-4z" fill="black"/>
        </svg>` :
        `<svg width="24" height="24" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
            <path d="M8 5v14l11-7z" fill="black"/>
        </svg>`;
}

// ============================================
// UI UPDATE FUNCTIONS
// ============================================

export function updateMainButtons() {
    if (!playBtn || !stopBtn) return;
    
    const playerState = getPlayerState();
    if (playerState.type === 'live' && playerState.isPlaying) {
        playBtn.style.display = 'none';
        stopBtn.style.display = 'flex';
    } else {
        playBtn.style.display = 'flex';
        stopBtn.style.display = 'none';
    }
}

export function updateFooter() {
    if (!playerFooter) return;
    
    const playerState = getPlayerState();
    
    if (playerState.isPlaying) {
        playerFooter.classList.add('visible');
        
        if (playerState.type === 'live') {
            // Hide time controls for live radio
            if (timeSlider) timeSlider.style.display = 'none';
            if (currentTimeDisplay) currentTimeDisplay.style.display = 'none';
            if (durationDisplay) durationDisplay.style.display = 'none';
        } else {
            // Show time controls for recordings
            if (timeSlider) timeSlider.style.display = 'block';
            if (currentTimeDisplay) currentTimeDisplay.style.display = 'block';
            if (durationDisplay) durationDisplay.style.display = 'block';
        }
        
        if (footerTitle) footerTitle.textContent = playerState.metadata.title;
        if (footerDate) footerDate.textContent = playerState.metadata.date;
    } else {
        playerFooter.classList.remove('visible');
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

    // Update media session
    if ('mediaSession' in navigator) {
        navigator.mediaSession.playbackState = 'paused';
    }
}

export function playRadio() {
    if (!mainRadio) return;
    
    // Stop any other audio first
    recordingPlayers.forEach(player => {
        if (!player.paused) {
            player.pause();
            player.currentTime = 0;
        }
    });

    // Force fresh stream each time
    let url = "https://radio.cabbage.town:8000/radio.mp3";
    if (navigator.userAgent.includes("Firefox")) {
        url += "?refresh=" + Date.now(); // bust cache
    }

    mainRadio.src = url;
    mainRadio.load();

    mainRadio.play().then(() => {
        setPlaying(true);
        setPlayerType('live');
        setAudioElement(mainRadio);
        
        // Update metadata from current now playing data
        const radioState = getRadioState();
        const nowPlaying = radioState.currentNowPlaying;
        
        if (nowPlaying) {
            if (nowPlaying.streamerDetails) {
                setPlayerMetadata({
                    title: `LIVE: ${nowPlaying.streamerDetails.name} w/ ${nowPlaying.streamerDetails.dj}`,
                    date: `${nowPlaying.artist} - ${nowPlaying.title}`
                });
            } else {
                setPlayerMetadata({
                    title: 'Live Radio',
                    date: `${nowPlaying.artist} - ${nowPlaying.title}`
                });
            }
        } else {
            setPlayerMetadata({
                title: 'Live Radio',
                date: 'cabbage.town'
            });
        }
        
        updateMainButtons();
        updateFooter();

        // Set up media session
        if ('mediaSession' in navigator) {
            const playerState = getPlayerState();
            navigator.mediaSession.metadata = new MediaMetadata({
                title: playerState.metadata.title,
                artist: 'cabbage.town',
                artwork: [
                    { src: '/the-cabbage.png', sizes: '512x512', type: 'image/png' }
                ]
            });

            // Add action handlers
            navigator.mediaSession.setActionHandler('play', () => {
                playRadio();
            });
            navigator.mediaSession.setActionHandler('pause', () => {
                stopRadio();
            });

            navigator.mediaSession.playbackState = 'playing';
        }
    }).catch(err => {
        console.warn("playback failed", err);
    });
}

export function updateLiveFooterMetadata(nowPlaying) {
    const playerState = getPlayerState();
    if (playerState.type === 'live' && playerState.isPlaying) {
        if (nowPlaying.streamerDetails) {
            setPlayerMetadata({
                title: `LIVE: ${nowPlaying.streamerDetails.name} w/ ${nowPlaying.streamerDetails.dj}`,
                date: `${nowPlaying.artist} - ${nowPlaying.title}`
            });
        } else {
            setPlayerMetadata({
                title: 'Live Radio',
                date: `${nowPlaying.artist} - ${nowPlaying.title}`
            });
        }
        
        if (footerTitle) footerTitle.textContent = playerState.metadata.title;
        if (footerDate) footerDate.textContent = playerState.metadata.date;
        
        // Update media session
        if ('mediaSession' in navigator) {
            navigator.mediaSession.metadata = new MediaMetadata({
                title: playerState.metadata.title,
                artist: 'cabbage.town',
                artwork: [
                    { src: '/the-cabbage.png', sizes: '512x512', type: 'image/png' }
                ]
            });
        }
    }
}

// ============================================
// RECORDING PLAYBACK FUNCTIONS
// ============================================

export function createRecordingPlayer(show, playButton) {
    let audioElement = null;

    return () => {
        if (!audioElement) {
            audioElement = document.createElement('audio');
            audioElement.style.display = 'none';

            audioElement.onplay = () => {
                // Stop live radio if playing
                const playerState = getPlayerState();
                if (playerState.type === 'live' && playerState.isPlaying) {
                    stopRadio();
                }

                // Stop other recording players
                recordingPlayers.forEach(player => {
                    if (player !== audioElement) {
                        player.pause();
                        player.currentTime = 0;
                    }
                });

                // Update unified player state
                setPlaying(true);
                setPlayerType('recording');
                setAudioElement(audioElement);
                setPlayerMetadata({
                    title: show.title,
                    date: show.date
                });
                
                updateMainButtons();
                updateFooter();
                updateFooterPlayButton(true);

                if ('mediaSession' in navigator) {
                    navigator.mediaSession.metadata = new MediaMetadata({
                        title: show.title,
                        artist: show.author || 'cabbage.town',
                        artwork: [
                            { src: '/the-cabbage.png', sizes: '512x512', type: 'image/png' }
                        ]
                    });
                    navigator.mediaSession.playbackState = 'playing';
                }

                playButton.innerHTML = `
                    <svg width="24" height="24" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
                        <path d="M6 19h4V5H6v14zm8-14v14h4V5h-4z" fill="white"/>
                    </svg>
                `;
            };

            audioElement.onpause = () => {
                setPlaying(false);
                updateFooterPlayButton(false);
                if ('mediaSession' in navigator) {
                    navigator.mediaSession.playbackState = 'paused';
                }
                playButton.innerHTML = `
                    <svg width="24" height="24" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
                        <path d="M8 5v14l11-7z" fill="white"/>
                    </svg>
                `;
            };

            audioElement.onloadedmetadata = () => {
                if (timeSlider) timeSlider.max = audioElement.duration;
                if (durationDisplay) durationDisplay.textContent = formatTime(audioElement.duration);
            };

            audioElement.ontimeupdate = () => {
                if (timeSlider && !timeSlider.dragging) {
                    timeSlider.value = audioElement.currentTime;
                    if (currentTimeDisplay) currentTimeDisplay.textContent = formatTime(audioElement.currentTime);
                }
            };

            audioElement.onended = () => {
                resetPlayer();
                updateFooter();
            };

            const source = document.createElement('source');
            source.src = show.recording.url;
            source.type = 'audio/mpeg';
            audioElement.appendChild(source);

            playButton.style.opacity = '0.5';

            audioElement.onloadeddata = () => {
                playButton.style.opacity = '1';
                audioElement.play();
            };

            audioElement.onerror = () => {
                playButton.style.opacity = '1';
                playButton.style.backgroundColor = '#d32f2f';
            };

            recordingPlayers.push(audioElement);
            document.body.appendChild(audioElement);
        } else {
            if (audioElement.paused) {
                audioElement.play();
            } else {
                audioElement.pause();
            }
        }
    };
}

// ============================================
// INITIALIZATION
// ============================================

export function initPlayer() {
    console.log('[Player] Initializing player module');
    
    // Get DOM references
    mainRadio = document.getElementById('radio');
    playerFooter = document.getElementById('playerFooter');
    footerPlayButton = document.getElementById('footerPlayButton');
    timeSlider = document.getElementById('timeSlider');
    currentTimeDisplay = document.getElementById('currentTime');
    durationDisplay = document.getElementById('duration');
    footerTitle = document.getElementById('footerTitle');
    footerDate = document.getElementById('footerDate');
    playBtn = document.getElementById('play');
    stopBtn = document.getElementById('stop');
    
    // Wire up live radio controls if they exist
    if (playBtn) {
        playBtn.onclick = () => {
            const playerState = getPlayerState();
            if (playerState.isPlaying && playerState.type === 'live') {
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
    
    // Footer play button controls both live and recording playback
    if (footerPlayButton) {
        footerPlayButton.onclick = () => {
            const playerState = getPlayerState();
            if (playerState.type === 'live') {
                if (playerState.isPlaying) {
                    stopRadio();
                } else {
                    playRadio();
                }
            } else if (playerState.type === 'recording' && playerState.audioElement) {
                if (playerState.audioElement.paused) {
                    playerState.audioElement.play();
                } else {
                    playerState.audioElement.pause();
                }
            }
        };
    }

    // Time slider controls for recordings
    if (timeSlider) {
        let isTimeSliderDragging = false;
        timeSlider.addEventListener('mousedown', () => {
            isTimeSliderDragging = true;
        });

        timeSlider.addEventListener('mouseup', () => {
            isTimeSliderDragging = false;
            const playerState = getPlayerState();
            if (playerState.type === 'recording' && playerState.audioElement) {
                playerState.audioElement.currentTime = timeSlider.value;
            }
        });
    }
    
    console.log('[Player] Player module initialized');
}

// Export recording players array for router
export function getRecordingPlayers() {
    return recordingPlayers;
}

