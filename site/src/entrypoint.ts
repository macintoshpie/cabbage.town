import type { Alpine } from 'alpinejs';

const STREAMER_MAP: Record<string, { name: string; dj: string }> = {
  'DJ Ted': { name: 'mulch channel', dj: 'dj ted' },
  'DJ Chicago Style': { name: 'IS WiLD hour', dj: 'DJ CHICAGO STYLE' },
  'Reginajingles': { name: 'The reginajingles show', dj: 'reginajingles' },
  'Nights Like These': { name: 'Late Nights Like These', dj: 'Nights Like These' },
  'the conductor': { name: 'tracks from terminus', dj: 'the conductor' },
};

interface PlayerStore {
  type: 'live' | 'recording' | null;
  isPlaying: boolean;
  metadata: { title: string; date: string };
  nowPlaying: {
    artist: string;
    title: string;
    art: string;
    streamerDetails?: { name: string; dj: string };
  } | null;
  currentTime: number;
  duration: number;
  _nowPlayingInterval: ReturnType<typeof setInterval> | null;
  _currentRecordingUrl: string | null;

  _getRadio(): HTMLAudioElement | null;
  _getRecording(): HTMLAudioElement | null;
  formatTime(seconds: number): string;
  playRadio(): void;
  stopRadio(): void;
  stopRecording(): void;
  stopAll(): void;
  _updateLiveMetadata(): void;
  _setupMediaSession(): void;
  playRecording(url: string, title: string, dj: string, date: string): void;
  _setupRecordingMediaSession(title: string, dj: string): void;
  togglePlayback(): void;
  seekTo(time: number): void;
  refreshNowPlaying(): Promise<void>;
  startNowPlayingPolling(): void;
  stopNowPlayingPolling(): void;
}

export default (Alpine: Alpine) => {
  const store: PlayerStore & ThisType<PlayerStore> = {
    // State
    type: null,
    isPlaying: false,
    metadata: { title: '', date: '' },
    nowPlaying: null,
    currentTime: 0,
    duration: 0,
    _nowPlayingInterval: null,
    _currentRecordingUrl: null,

    // Audio element accessors
    _getRadio() {
      return document.getElementById('radio') as HTMLAudioElement | null;
    },
    _getRecording() {
      return document.getElementById('recordingPlayer') as HTMLAudioElement | null;
    },

    // Format seconds to m:ss
    formatTime(seconds: number) {
      const m = Math.floor(seconds / 60);
      const s = Math.floor(seconds % 60);
      return `${m}:${s.toString().padStart(2, '0')}`;
    },

    // ---- Live Radio ----

    playRadio() {
      const radio = this._getRadio();
      const recording = this._getRecording();
      if (!radio) return;

      // Stop recording if playing
      if (recording && !recording.paused) {
        recording.pause();
        recording.currentTime = 0;
      }

      let url = 'https://radio.cabbage.town:8000/radio.mp3';
      if (navigator.userAgent.includes('Firefox')) {
        url += '?refresh=' + Date.now();
      }

      radio.src = url;
      radio.load();
      radio.play()
        .then(() => {
          this.type = 'live';
          this.isPlaying = true;
          this._currentRecordingUrl = null;
          this._updateLiveMetadata();
          this._setupMediaSession();
        })
        .catch((err: unknown) => console.warn('playback failed', err));

      // Auto-reconnect on network error
      radio.onerror = () => {
        const err = radio.error;
        if (err?.code === MediaError.MEDIA_ERR_NETWORK) {
          console.log('stream error, retrying in 5s...');
          setTimeout(() => this.playRadio(), 5000);
        }
      };
    },

    stopRadio() {
      const radio = this._getRadio();
      if (!radio) return;
      radio.pause();
      radio.src = '';
      radio.load();
      radio.onerror = null;

      this.type = null;
      this.isPlaying = false;
      this.metadata = { title: '', date: '' };

      if ('mediaSession' in navigator) {
        navigator.mediaSession.playbackState = 'paused';
      }
    },

    stopRecording() {
      const recording = this._getRecording();
      if (!recording) return;
      recording.pause();
      recording.currentTime = 0;
      recording.src = '';
      recording.load();
      this.type = null;
      this.isPlaying = false;
      this._currentRecordingUrl = null;
      this.metadata = { title: '', date: '' };
      this.currentTime = 0;
      this.duration = 0;
      if ('mediaSession' in navigator) {
        navigator.mediaSession.playbackState = 'paused';
      }
    },

    stopAll() {
      if (this.type === 'live') {
        this.stopRadio();
      } else if (this.type === 'recording') {
        this.stopRecording();
      }
    },

    _updateLiveMetadata() {
      if (this.nowPlaying?.streamerDetails) {
        this.metadata = {
          title: `LIVE: ${this.nowPlaying.streamerDetails.name} w/ ${this.nowPlaying.streamerDetails.dj}`,
          date: `${this.nowPlaying.artist} - ${this.nowPlaying.title}`,
        };
      } else if (this.nowPlaying) {
        this.metadata = {
          title: 'Live Radio',
          date: `${this.nowPlaying.artist} - ${this.nowPlaying.title}`,
        };
      } else {
        this.metadata = { title: 'Live Radio', date: 'cabbage.town' };
      }
    },

    _setupMediaSession() {
      if (!('mediaSession' in navigator)) return;
      navigator.mediaSession.metadata = new MediaMetadata({
        title: this.metadata.title,
        artist: 'cabbage.town',
        artwork: [{ src: '/the-cabbage.png', sizes: '512x512', type: 'image/png' }],
      });
      navigator.mediaSession.setActionHandler('play', () => this.playRadio());
      navigator.mediaSession.setActionHandler('pause', () => this.stopRadio());
      navigator.mediaSession.playbackState = 'playing';
    },

    // ---- Recording Playback ----

    playRecording(url: string, title: string, dj: string, date: string) {
      const recording = this._getRecording();
      if (!recording) return;

      // If same recording, toggle pause/play
      if (this._currentRecordingUrl === url && this.type === 'recording') {
        if (this.isPlaying) {
          recording.pause();
        } else {
          recording.play();
        }
        return;
      }

      // Stop live radio if playing
      if (this.type === 'live' && this.isPlaying) {
        this.stopRadio();
      }

      this._currentRecordingUrl = url;
      recording.src = url;
      recording.load();

      recording.onloadeddata = () => recording.play();

      recording.onplay = () => {
        this.type = 'recording';
        this.isPlaying = true;
        this.metadata = { title, date: `${dj} \u2014 ${date}` };
        this.duration = recording.duration || 0;
        this._setupRecordingMediaSession(title, dj);
      };

      recording.onpause = () => {
        if (this.type === 'recording') {
          this.isPlaying = false;
          if ('mediaSession' in navigator) {
            navigator.mediaSession.playbackState = 'paused';
          }
        }
      };

      recording.ontimeupdate = () => {
        if (this.type === 'recording') {
          this.currentTime = recording.currentTime;
        }
      };

      recording.onloadedmetadata = () => {
        if (this.type === 'recording') {
          this.duration = recording.duration;
        }
      };

      recording.onended = () => {
        this.type = null;
        this.isPlaying = false;
        this._currentRecordingUrl = null;
        this.metadata = { title: '', date: '' };
        this.currentTime = 0;
        this.duration = 0;
      };

      recording.onerror = () => {
        console.error('Failed to load recording:', url);
      };
    },

    _setupRecordingMediaSession(title: string, dj: string) {
      if (!('mediaSession' in navigator)) return;
      navigator.mediaSession.metadata = new MediaMetadata({
        title,
        artist: dj || 'cabbage.town',
        artwork: [{ src: '/the-cabbage.png', sizes: '512x512', type: 'image/png' }],
      });
      navigator.mediaSession.setActionHandler('play', () => {
        this._getRecording()?.play();
      });
      navigator.mediaSession.setActionHandler('pause', () => {
        this._getRecording()?.pause();
      });
      navigator.mediaSession.playbackState = 'playing';
    },

    // ---- Footer ----

    togglePlayback() {
      if (this.type === 'live') {
        if (this.isPlaying) this.stopRadio();
        else this.playRadio();
      } else if (this.type === 'recording') {
        const recording = this._getRecording();
        if (this.isPlaying) recording?.pause();
        else recording?.play();
      }
    },

    seekTo(time: number) {
      const recording = this._getRecording();
      if (recording && this.type === 'recording') {
        recording.currentTime = time;
      }
    },

    // ---- Now Playing ----

    async refreshNowPlaying() {
      try {
        const res = await fetch('https://radio.cabbage.town/api/nowplaying/cabbage.town');
        if (!res.ok) return;
        const data = await res.json();
        const live = data.live;
        let streamerDetails: { name: string; dj: string } | undefined;
        if (live.is_live) {
          streamerDetails = STREAMER_MAP[live.streamer_name] || {
            name: live.streamer_name,
            dj: live.streamer_name,
          };
        }
        const song = data.now_playing.song;
        this.nowPlaying = {
          artist: song.artist,
          title: song.title,
          art: song.art,
          streamerDetails,
        };
        // Update footer if live radio is playing
        if (this.type === 'live' && this.isPlaying) {
          this._updateLiveMetadata();
        }
      } catch (e) {
        console.warn('Failed to fetch now playing', e);
      }
    },

    startNowPlayingPolling() {
      if (this._nowPlayingInterval) return; // already polling
      this.refreshNowPlaying();
      this._nowPlayingInterval = setInterval(() => this.refreshNowPlaying(), 15000);
    },

    stopNowPlayingPolling() {
      if (this._nowPlayingInterval) {
        clearInterval(this._nowPlayingInterval);
        this._nowPlayingInterval = null;
      }
    },
  };

  Alpine.store('player', store);
};
