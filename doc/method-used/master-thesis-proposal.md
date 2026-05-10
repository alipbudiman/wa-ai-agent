---
title: "Master Thesis/Skripsi Proposal Analysis"
category: "methodology"
last_analyzed: "2026-05-03"
---

# Analisis Metodologi Penelitian: Implementasi WaNexus AI Agent

Jika proyek **WaNexus AI Agent** ini dijadikan sebagai topik skripsi atau tesis informatika, maka metodologi yang paling tepat adalah **Design Science Research (DSR)** yang dikombinasikan dengan **Metode Eksperimental** pada tahap evaluasi.

---

## 🏗️ 1. Metodologi Utama: Design Science Research (DSR)

DSR adalah paradigma penelitian yang berfokus pada penciptaan "artefak" (sistem, model, atau framework) untuk memecahkan masalah praktis. Karena WaNexus adalah sistem agentic yang kompleks, DSR memungkinkan Anda untuk menonjolkan inovasi pada arsitekturnya.

### Tahapan DSR untuk WaNexus:
1. **Problem Identification**: Masalah efisiensi pengerjaan tugas engineering yang kompleks dan tingginya tingkat halusinasi pada LLM standar.
2. **Objectives for a Solution**: Membangun sistem AI yang otonom, memiliki akses ke tools (ReAct), dan berbasis data (RAG) untuk meminimalisir kesalahan.
3. **Design & Development**: Tahap di mana Anda merancang arsitektur **Dual-Core (Go/Python)** dan implementasi **LangGraph** sebagai pengatur state.
4. **Demonstration**: Menguji sistem untuk menyelesaikan kasus nyata (misal: membangun sebuah fitur aplikasi secara otomatis menggunakan Code Agent).
5. **Evaluation**: Mengukur efektivitas artefak menggunakan metrik kuantitatif.

---

## 🔬 2. Metode Evaluasi: Eksperimental

Pada tahap evaluasi (tahap 5 di DSR), Anda dapat menggunakan metode eksperimen untuk memberikan "bobot ilmiah" pada penelitian Anda.

### Skenario Eksperimen yang Bisa Digunakan:
- **A/B Testing**: Membandingkan akurasi respons **Reply Agent (Sofia)** yang menggunakan RAG vs. LLM standar tanpa RAG.
- **Success Rate Analysis**: Mengukur tingkat keberhasilan **Code Agent** dalam menyelesaikan 50 variasi tugas pemrograman (menggunakan dataset seperti HumanEval).
- **Latency vs. Accuracy**: Menganalisis pengaruh penggunaan model lokal (Ollama) vs. Cloud (Anthropic) terhadap kecepatan respon dan kualitas solusi.

---

## 📝 3. Struktur Judul Penelitian (Contoh)

Berikut adalah beberapa inspirasi judul yang bisa digunakan:

1. **"Rancang Bangun Multi-Agent System Menggunakan Framework ReAct dan LangGraph untuk Otomasi Tugas Software Engineering"**
   *(Fokus pada Arsitektur)*
2. **"Implementasi Retrieval-Augmented Generation (RAG) dan Semantic Routing pada AI Agent Berbasis WhatsApp untuk Meningkatkan Akurasi Informasi"**
   *(Fokus pada RAG & Data)*
3. **"Analisis Performa Arsitektur Hybrid Edge-Cloud pada Orchestration AI Agent Menggunakan Ollama dan Anthropic API"**
   *(Fokus pada Infrastruktur)*

---

## 💡 4. Keunggulan Ilmiah (Novelty) WaNexus
Untuk skripsi/tesis, Anda harus menonjolkan apa yang baru atau unik. Pada WaNexus, poin uniknya adalah:
- **Dual-Core Architecture**: Pemisahan runtime (Go) dan reasoning (Python) untuk skalabilitas.
- **Human-in-the-Loop Integration**: Mekanisme keamanan yang menginterupsi proses otomatisasi untuk persetujuan manusia.
- **Tool Coercion**: Kemampuan sistem untuk bekerja dengan model yang tidak mendukung native function calling.

---
## Related Documents
- [Master Method Analysis](master-method.md)
- [Architecture Master](../architecture/master-architecture.md)
- [Agents Master](../agents/master-agents.md)
