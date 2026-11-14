//! Sürecin harcadığı işlemci süresi ve tepe bellek kullanımı (Go tarafı aynı
//! Windows çağrılarını kullanır, ki iki dilin sayıları karşılaştırılabilsin).

/// Sürecin şimdiye dek harcadığı toplam işlemci süresi (kullanıcı + çekirdek), saniye.
#[cfg(windows)]
pub fn islemci_sn() -> f64 {
    use windows_sys::Win32::Foundation::FILETIME;
    use windows_sys::Win32::System::Threading::{GetCurrentProcess, GetProcessTimes};
    let bos = FILETIME { dwLowDateTime: 0, dwHighDateTime: 0 };
    let (mut olus, mut cikis, mut cekirdek, mut kullanici) = (bos, bos, bos, bos);
    let tamam = unsafe { GetProcessTimes(GetCurrentProcess(), &mut olus, &mut cikis, &mut cekirdek, &mut kullanici) };
    if tamam == 0 {
        return 0.0;
    }
    let sn = |f: FILETIME| ((f.dwHighDateTime as u64) << 32 | f.dwLowDateTime as u64) as f64 / 1e7;
    sn(cekirdek) + sn(kullanici)
}

#[cfg(not(windows))]
pub fn islemci_sn() -> f64 {
    0.0
}

/// Sürecin şimdiye dek ulaştığı en yüksek çalışma kümesi (fiziksel bellek), MB.
#[cfg(windows)]
pub fn tepe_bellek_mb() -> f64 {
    use windows_sys::Win32::System::ProcessStatus::{K32GetProcessMemoryInfo, PROCESS_MEMORY_COUNTERS};
    use windows_sys::Win32::System::Threading::GetCurrentProcess;
    let mut sayac: PROCESS_MEMORY_COUNTERS = unsafe { std::mem::zeroed() };
    sayac.cb = std::mem::size_of::<PROCESS_MEMORY_COUNTERS>() as u32;
    let tamam = unsafe { K32GetProcessMemoryInfo(GetCurrentProcess(), &mut sayac, sayac.cb) };
    if tamam == 0 {
        return 0.0;
    }
    sayac.PeakWorkingSetSize as f64 / (1024.0 * 1024.0)
}

#[cfg(not(windows))]
pub fn tepe_bellek_mb() -> f64 {
    0.0
}
