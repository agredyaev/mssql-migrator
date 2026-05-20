use crate::config::Config;
use crate::driver::timing::TimingConn;
use crate::error::{Error, Result};
use crate::sql;

pub async fn acquire(conn: &mut TimingConn, cfg: &Config) -> Result<()> {
    let ms = cfg.lock_timeout.as_millis() as i32;
    let ms_str = ms.to_string();
    let rows = conn.query(sql::lock::ACQUIRE, &[ms_str.as_str()]).await?;
    let code = rows.first().and_then(|r| r.get_i32(0)).unwrap_or(-1);
    if code < 0 {
        return Err(Error::LockTimeout);
    }
    Ok(())
}

pub async fn release(conn: &mut TimingConn) -> Result<()> {
    let _ = conn.query(sql::lock::RELEASE, &[]).await?;
    Ok(())
}
